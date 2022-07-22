## server side

$ ssh-p2p newkey
$ ssh-p2p server -key=$KEY -dial=127.0.0.1:22

share $KEY value to client side

## client side

$ KEY=xxxxxxxx-xxxx-xxxx-xxxxxxxx
$ ssh-p2p client -key=$KEY -listen=127.0.0.1:2222

## client side other terminal

$ ssh -p 2222 127.0.0.1

**connect to server side sshd !!**
