package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/nobonobo/ssh-p2p/signaling"
	"github.com/pion/ice/v2"
	"github.com/pion/logging"
	"github.com/pion/turn/v2"
	"github.com/pion/webrtc/v3"
)

const usage = `Usage: ssh-p2p SUBCMD [options]
sub-commands:
	newkey
		new generate key of connection
	server -key="..." [-dial="127.0.0.1:22"]
		ssh server side peer mode
	client -key="..." [-listen="127.0.0.1:2222"]
		ssh client side peer mode
`

var (
	defaultRTCConfiguration = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
					"stun:stun2.l.google.com:19302",
					"stun:stun3.l.google.com:19302",
					"stun:stun4.l.google.com:19302",
				},
			},
		},
	}
)

// Encode encodes the input in base64
// It can optionally zip the input before encoding
func Encode(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

// Decode decodes the input from base64
// It can optionally unzip the input after decoding
func Decode(in string, obj interface{}) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, obj)
	if err != nil {
		panic(err)
	}
}

func push(dst, src, sdp string) error {
	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(signaling.ConnectInfo{
		Source: src,
		SDP:    sdp,
	}); err != nil {
		return err
	}
	resp, err := http.Post(signaling.URI()+path.Join("/", "push", dst), "application/json", buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http failed")
	}
	return nil
}

func pull(ctx context.Context, id string) <-chan signaling.ConnectInfo {
	ch := make(chan signaling.ConnectInfo)
	var retry time.Duration
	go func() {
		faild := func() {
			if retry < 10 {
				retry++
			}
			time.Sleep(retry * time.Second)
		}
		defer close(ch)
		for {
			req, err := http.NewRequest("GET", signaling.URI()+path.Join("/", "pull", id), nil)
			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				log.Println("get failed:", err)
				faild()
				continue
			}
			req = req.WithContext(ctx)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				log.Println("get failed:", err)
				faild()
				continue
			}
			defer res.Body.Close()
			retry = time.Duration(0)
			var info signaling.ConnectInfo
			if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
				if err == io.EOF {
					continue
				}
				if ctx.Err() == context.Canceled {
					return
				}
				log.Println("get failed:", err)
				faild()
				continue
			}
			if len(info.Source) > 0 && len(info.SDP) > 0 {
				ch <- info
			}
		}
	}()
	return ch
}

func doPingTest(client *turn.Client, relayConn net.PacketConn) error {
	// Send BindingRequest to learn our external IP
	mappedAddr, err := client.SendBindingRequest()
	if err != nil {
		return err
	}

	// Set up pinger socket (pingerConn)
	pingerConn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}
	defer func() {
		if closeErr := pingerConn.Close(); closeErr != nil {
			panic(closeErr)
		}
	}()

	// Punch a UDP hole for the relayConn by sending a data to the mappedAddr.
	// This will trigger a TURN client to generate a permission request to the
	// TURN server. After this, packets from the IP address will be accepted by
	// the TURN server.
	_, err = relayConn.WriteTo([]byte("Hello"), mappedAddr)
	if err != nil {
		return err
	}

	// Start read-loop on pingerConn
	go func() {
		buf := make([]byte, 1600)
		for {
			n, from, pingerErr := pingerConn.ReadFrom(buf)
			if pingerErr != nil {
				break
			}

			msg := string(buf[:n])
			if sentAt, pingerErr := time.Parse(time.RFC3339Nano, msg); pingerErr == nil {
				rtt := time.Since(sentAt)
				log.Printf("%d bytes from from %s time=%d ms\n", n, from.String(), int(rtt.Seconds()*1000))
			}
		}
	}()

	// Start read-loop on relayConn
	go func() {
		buf := make([]byte, 1600)
		for {
			n, from, readerErr := relayConn.ReadFrom(buf)
			if readerErr != nil {
				break
			}

			// Echo back
			if _, readerErr = relayConn.WriteTo(buf[:n], from); readerErr != nil {
				break
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)

	// Send 10 packets from relayConn to the echo server
	for i := 0; i < 10; i++ {
		msg := time.Now().Format(time.RFC3339Nano)
		_, err = pingerConn.WriteTo([]byte(msg), relayConn.LocalAddr())
		if err != nil {
			return err
		}

		// For simplicity, this example does not wait for the pong (reply).
		// Instead, sleep 1 second.
		time.Sleep(time.Second)
	}

	return nil
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Println("failed to load env file")
	}
	if urlstr := os.Getenv("STUN_SERVER_URLS"); urlstr != "" {
		ices := defaultRTCConfiguration.ICEServers
		for _, url := range strings.Split(urlstr, " ") {
			ices = append(ices, webrtc.ICEServer{
				URLs: []string{url},
			})
		}
		defaultRTCConfiguration.ICEServers = ices
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var flags *flag.FlagSet
	flags = flag.NewFlagSet("", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, usage)
		flags.PrintDefaults()
		os.Exit(1)
	}

	switch cmd {
	default:
		flags.Usage()
	case "newkey":
		key := uuid.New().String()
		fmt.Println(key)
		os.Exit(0)
	case "server":
		var addr, key string
		flags.StringVar(&addr, "dial", "127.0.0.1:22", "dial addr = host:port")
		flags.StringVar(&key, "key", "sample", "connection key")
		if err := flags.Parse(os.Args[2:]); err != nil {
			log.Fatalln(err)
		}

		// TURN client won't create a local listening socket by itself.
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		if err != nil {
			panic(err)
		}
		defer func() {
			if closeErr := conn.Close(); closeErr != nil {
				panic(closeErr)
			}
		}()

		turnServerAddr := fmt.Sprintf("%s:%d", "51.68.174.6", 3478)

		cfg := &turn.ClientConfig{
			STUNServerAddr: turnServerAddr,
			TURNServerAddr: turnServerAddr,
			Conn:           conn,
			Username:       "foo",
			Password:       "bar",
			Realm:          "pion.ly",
			LoggerFactory:  logging.NewDefaultLoggerFactory(),
		}

		client, err := turn.NewClient(cfg)
		if err != nil {
			panic(err)
		}
		defer client.Close()

		// Start listening on the conn provided.
		err = client.Listen()
		if err != nil {
			panic(err)
		}

		// Allocate a relay socket on the TURN server. On success, it
		// will return a net.PacketConn which represents the remote
		// socket.
		relayConn, err := client.Allocate()
		if err != nil {
			panic(err)
		}
		defer func() {
			if closeErr := relayConn.Close(); closeErr != nil {
				panic(closeErr)
			}
		}()

		// The relayConn's local address is actually the transport
		// address assigned on the TURN server.
		log.Printf("relayed-address=%s", relayConn.LocalAddr().String())

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT)
		ctx, cancel := context.WithCancel(context.Background())
		go serve(ctx, key, addr)
		<-sig
		cancel()
	case "client":
		var addr, key string
		flags.StringVar(&addr, "listen", "127.0.0.1:2222", "listen addr = host:port")
		flags.StringVar(&key, "key", "sample", "connection key")

		// TURN client won't create a local listening socket by itself.
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		if err != nil {
			panic(err)
		}
		defer func() {
			if closeErr := conn.Close(); closeErr != nil {
				panic(closeErr)
			}
		}()

		turnServerAddr := fmt.Sprintf("%s:%d", "51.68.174.6", 3478)

		cfg := &turn.ClientConfig{
			STUNServerAddr: turnServerAddr,
			TURNServerAddr: turnServerAddr,
			Conn:           conn,
			Username:       "foo",
			Password:       "bar",
			Realm:          "pion.ly",
			LoggerFactory:  logging.NewDefaultLoggerFactory(),
		}

		client, err := turn.NewClient(cfg)
		if err != nil {
			panic(err)
		}
		defer client.Close()

		// Start listening on the conn provided.
		err = client.Listen()
		if err != nil {
			panic(err)
		}

		// Allocate a relay socket on the TURN server. On success, it
		// will return a net.PacketConn which represents the remote
		// socket.
		relayConn, err := client.Allocate()
		if err != nil {
			panic(err)
		}
		defer func() {
			if closeErr := relayConn.Close(); closeErr != nil {
				panic(closeErr)
			}
		}()

		// The relayConn's local address is actually the transport
		// address assigned on the TURN server.
		log.Printf("relayed-address=%s", relayConn.LocalAddr().String())

		//err = doPingTest(client, relayConn)
		//if err != nil {
		//	panic(err)
		//}

		if err := flags.Parse(os.Args[2:]); err != nil {
			log.Fatalln(err)
		}
		l, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalln(err)
		}
		log.Println("listen:", addr)
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			for {
				sock, err := l.Accept()
				if err != nil {
					log.Println(err)
					continue
				}
				go connect(ctx, key, sock)
			}
		}()
		<-sig
		cancel()
	}
}

type sendWrap struct {
	*webrtc.DataChannel
}

func (s *sendWrap) Write(b []byte) (int, error) {
	err := s.DataChannel.Send(b)
	return len(b), err
}

func serve(ctx context.Context, key, addr string) {
	log.Println("server started")
	for v := range pull(ctx, key) {
		log.Printf("info: %#v", v)
		pc, err := webrtc.NewPeerConnection(defaultRTCConfiguration)
		if err != nil {
			log.Println("rtc error:", err)
			continue
		}
		ssh, err := net.Dial("tcp", addr)
		if err != nil {
			log.Println("ssh dial filed:", err)
			pc.Close()
			continue
		}
		pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			log.Print("pc ice state change:", state)
			if state == ice.ConnectionStateDisconnected {
				pc.Close()
				ssh.Close()
			}
		})
		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			//dc.Lock()
			dc.OnOpen(func() {
				log.Print("dial:", addr)
				io.Copy(&sendWrap{dc}, ssh)
				log.Println("disconnected")
			})
			dc.OnMessage(func(payload webrtc.DataChannelMessage) {
				_, err := ssh.Write(payload.Data)
				if err != nil {
					log.Println("ssh write failed:", err)
					pc.Close()
					return
				}
			})
			//dc.Unlock()
		})
		offer := webrtc.SessionDescription{}
		Decode(v.SDP, &offer)
		if err := pc.SetRemoteDescription(offer); err != nil {
			log.Println("rtc error:", err)
			pc.Close()
			ssh.Close()
			continue
		}
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			log.Println("rtc error:", err)
			pc.Close()
			ssh.Close()
			continue
		}
		// Create channel that is blocked until ICE Gathering is complete
		gatherComplete := webrtc.GatheringCompletePromise(pc)

		// Sets the LocalDescription, and starts our UDP listeners
		err = pc.SetLocalDescription(answer)
		if err != nil {
			panic(err)
		}
		// Block until ICE Gathering is complete, disabling trickle ICE
		// we do this because we only can exchange one signaling message
		// in a production application you should exchange ICE Candidates via OnICECandidate
		<-gatherComplete
		if err := push(v.Source, key, Encode(*pc.LocalDescription())); err != nil {
			log.Println("rtc error:", err)
			pc.Close()
			ssh.Close()
			continue
		}
	}
}

func connect(ctx context.Context, key string, sock net.Conn) {
	id := uuid.New().String()
	log.Println("client id:", id)
	pc, err := webrtc.NewPeerConnection(defaultRTCConfiguration)
	if err != nil {
		log.Println("rtc error:", err)
		return
	}
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Print("pc ice state change:", state)
	})
	dc, err := pc.CreateDataChannel("data", nil)
	if err != nil {
		log.Println("create dc failed:", err)
		pc.Close()
		return
	}
	//dc.Lock()
	dc.OnOpen(func() {
		io.Copy(&sendWrap{dc}, sock)
		pc.Close()
		log.Println("disconnected")
	})
	dc.OnMessage(func(payload webrtc.DataChannelMessage) {
		_, err := sock.Write(payload.Data)
		if err != nil {
			log.Println("sock write failed:", err)
			pc.Close()
			return
		}
	})
	//dc.Unlock()
	log.Printf("DataChannel:%+v", dc)
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for v := range pull(ctx, id) {
			log.Printf("info: %#v", v)
			answer := webrtc.SessionDescription{}
			Decode(v.SDP, &answer)
			if err := pc.SetRemoteDescription(answer); err != nil {
				log.Println("rtc error:", err)
				pc.Close()
				return
			}
			return
		}
	}()
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Println("create offer error:", err)
		pc.Close()
		return
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	err = pc.SetLocalDescription(offer)
	if err != nil {
		panic(err)
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete
	if err := push(key, id, Encode(*pc.LocalDescription())); err != nil {
		log.Println("push error:", err)
		pc.Close()
		return
	}
}
