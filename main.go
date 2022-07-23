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
					"stun.12connect.com:3478",
					"stun:stun.12voip.com:3478",
					"stun:stun.1und1.de:3478",
					"stun:stun.3cx.com:3478",
					"stun:stun.acrobits.cz:3478",
					"stun:stun.actionvoip.com:3478",
					"stun:stun.advfn.com:3478",
					"stun:stun.altar.com.pl:3478",
					"stun:stun.antisip.com:3478",
					"stun:stun.avigora.fr:3478",
					"stun:stun.bluesip.net:3478",
					"stun:stun.cablenet-as.net:3478",
					"stun:stun.callromania.ro:3478",
					"stun:stun.callwithus.com:3478",
					"stun:stun.cheapvoip.com:3478",
					"stun:stun.cloopen.com:3478",
					"stun:stun.commpeak.com:3478",
					"stun:stun.cope.es:3478",
					"stun:stun.counterpath.com:3478",
					"stun:stun.counterpath.net:3478",
					"stun:stun.dcalling.de:3478",
					"stun:stun.demos.ru:3478",
					"stun:stun.dus.net:3478",
					"stun:stun.easycall.pl:3478",
					"stun:stun.easyvoip.com:3478",
					"stun:stun.ekiga.net:3478",
					"stun:stun.epygi.com:3478",
					"stun:stun.etoilediese.fr:3478",
					"stun:stun.faktortel.com.au:3478",
					"stun:stun.freecall.com:3478",
					"stun:stun.freeswitch.org:3478",
					"stun:stun.freevoipdeal.com:3478",
					"stun:stun.gmx.de:3478",
					"stun:stun.gmx.net:3478",
					"stun:stun.halonet.pl:3478",
					"stun:stun.hoiio.com:3478",
					"stun:stun.hosteurope.de:3478",
					"stun:stun.infra.net:3478",
					"stun:stun.internetcalls.com:3478",
					"stun:stun.intervoip.com:3478",
					"stun:stun.ipfire.org:3478",
					"stun:stun.ippi.fr:3478",
					"stun:stun.ipshka.com:3478",
					"stun:stun.it1.hr:3478",
					"stun:stun.ivao.aero:3478",
					"stun:stun.jumblo.com:3478",
					"stun:stun.justvoip.com:3478",
					"stun:stun.l.google.com:19302",
					"stun:stun.linphone.org:3478",
					"stun:stun.liveo.fr:3478",
					"stun:stun.lowratevoip.com:3478",
					"stun:stun.lundimatin.fr:3478",
					"stun:stun.mit.de:3478",
					"stun:stun.miwifi.com:3478",
					"stun:stun.modulus.gr:3478",
					"stun:stun.myvoiptraffic.com:3478",
					"stun:stun.netappel.com:3478",
					"stun:stun.netgsm.com.tr:3478",
					"stun:stun.nfon.net:3478",
					"stun:stun.nonoh.net:3478",
					"stun:stun.nottingham.ac.uk:3478",
					"stun:stun.ooma.com:3478",
					"stun:stun.ozekiphone.com:3478",
					"stun:stun.pjsip.org:3478",
					"stun:stun.poivy.com:3478",
					"stun:stun.powervoip.com:3478",
					"stun:stun.ppdi.com:3478",
					"stun:stun.qq.com:3478",
					"stun:stun.rackco.com:3478",
					"stun:stun.rockenstein.de:3478",
					"stun:stun.rolmail.net:3478",
					"stun:stun.rynga.com:3478",
					"stun:stun.schlund.de:3478",
					"stun:stun.sigmavoip.com:3478",
					"stun:stun.sip.us:3478",
					"stun:stun.sipdiscount.com:3478",
					"stun:stun.sipgate.net:10000",
					"stun:stun.sipgate.net:3478",
					"stun:stun.siplogin.de:3478",
					"stun:stun.sipnet.net:3478",
					"stun:stun.sipnet.ru:3478",
					"stun:stun.sippeer.dk:3478",
					"stun:stun.siptraffic.com:3478",
					"stun:stun.sma.de:3478",
					"stun:stun.smartvoip.com:3478",
					"stun:stun.smsdiscount.com:3478",
					"stun:stun.solcon.nl:3478",
					"stun:stun.solnet.ch:3478",
					"stun:stun.sonetel.com:3478",
					"stun:stun.sonetel.net:3478",
					"stun:stun.sovtest.ru:3478",
					"stun:stun.srce.hr:3478",
					"stun:stun.stunprotocol.org:3478",
					"stun:stun.t-online.de:3478",
					"stun:stun.tel.lu:3478",
					"stun:stun.telbo.com:3478",
					"stun:stun.tng.de:3478",
					"stun:stun.twt.it:3478",
					"stun:stun.uls.co.za:3478",
					"stun:stun.unseen.is:3478",
					"stun:stun.usfamily.net:3478",
					"stun:stun.viva.gr:3478",
					"stun:stun.vivox.com:3478",
					"stun:stun.vo.lu:3478",
					"stun:stun.voicetrading.com:3478",
					"stun:stun.voip.aebc.com:3478",
					"stun:stun.voip.blackberry.com:3478",
					"stun:stun.voip.eutelia.it:3478",
					"stun:stun.voipblast.com:3478",
					"stun:stun.voipbuster.com:3478",
					"stun:stun.voipbusterpro.com:3478",
					"stun:stun.voipcheap.co.uk:3478",
					"stun:stun.voipcheap.com:3478",
					"stun:stun.voipgain.com:3478",
					"stun:stun.voipgate.com:3478",
					"stun:stun.voipinfocenter.com:3478",
					"stun:stun.voipplanet.nl:3478",
					"stun:stun.voippro.com:3478",
					"stun:stun.voipraider.com:3478",
					"stun:stun.voipstunt.com:3478",
					"stun:stun.voipwise.com:3478",
					"stun:stun.voipzoom.com:3478",
					"stun:stun.voys.nl:3478",
					"stun:stun.voztele.com:3478",
					"stun:stun.webcalldirect.com:3478",
					"stun:stun.wifirst.net:3478",
					"stun:stun.xtratelecom.es:3478",
					"stun:stun.zadarma.com:3478",
					"stun:stun1.faktortel.com.au:3478",
					"stun:stun1.l.google.com:19302",
					"stun:stun2.l.google.com:19302",
					"stun:stun3.l.google.com:19302",
					"stun:stun4.l.google.com:19302",
					"stun:stun.nextcloud.com:443",
					"stun:relay.webwormhole.io:3478",
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
