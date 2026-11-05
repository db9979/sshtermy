package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"github.com/pkg/sftp"
	"io"
	"log"
	"net"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

func main() {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:2222")
	if err != nil {
		log.Fatal(err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(c, cfg)
	}
}

func handle(c net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	go func() {
		for r := range reqs {
			if r.Type == "tcpip-forward" {
				r.Reply(false, nil)
				continue
			}
			if r.WantReply {
				r.Reply(true, nil)
			}
		}
	}()
	for nc := range chans {
		switch nc.ChannelType() {
		case "session":
			go session(nc)
		case "direct-tcpip":
			go direct(nc)
		default:
			nc.Reject(ssh.UnknownChannelType, "no")
		}
	}
	_ = sc
}

func direct(nc ssh.NewChannel) {
	var d struct {
		Host  string
		Port  uint32
		OHost string
		OPort uint32
	}
	ssh.Unmarshal(nc.ExtraData(), &d)
	out, err := net.Dial("tcp", net.JoinHostPort(d.Host, itoa(d.Port)))
	if err != nil {
		nc.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	ch, reqs, _ := nc.Accept()
	go ssh.DiscardRequests(reqs)
	go func() { io.Copy(out, ch); out.Close() }()
	io.Copy(ch, out)
	ch.Close()
}

func itoa(p uint32) string {
	return string([]byte(func() []byte {
		b := []byte{}
		for p > 0 {
			b = append([]byte{byte('0' + p%10)}, b...)
			p /= 10
		}
		return b
	}()))
}

func session(nc ssh.NewChannel) {
	ch, reqs, _ := nc.Accept()
	var f *os.File
	var cols, rows uint32 = 80, 24
	for r := range reqs {
		switch r.Type {
		case "pty-req":
			l := binary.BigEndian.Uint32(r.Payload)
			p := r.Payload[4+l:]
			cols, rows = binary.BigEndian.Uint32(p), binary.BigEndian.Uint32(p[4:])
			r.Reply(true, nil)
		case "window-change":
			cols, rows = binary.BigEndian.Uint32(r.Payload), binary.BigEndian.Uint32(r.Payload[4:])
			if f != nil {
				pty.Setsize(f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
			}
		case "shell":
			cmd := exec.Command("bash", "--norc", "-i")
			cmd.Env = append(os.Environ(), "TERM=xterm-256color", "PS1=\\[\\e[1;32m\\]dominik@dgx-spark\\[\\e[0m\\]:\\[\\e[1;34m\\]~\\[\\e[0m\\]$ ")
			cmd.Dir = "/home/claude"
			f, _ = pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
			r.Reply(true, nil)
			go io.Copy(f, ch)
			go func() {
				io.Copy(ch, f)
				cmd.Wait()
				ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
				ch.Close()
			}()
		case "subsystem":
			l := binary.BigEndian.Uint32(r.Payload)
			if string(r.Payload[4:4+l]) != "sftp" {
				r.Reply(false, nil)
				continue
			}
			r.Reply(true, nil)
			go func() {
				srv, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory("/tmp/sftp-home"))
				if err == nil {
					srv.Serve()
					srv.Close()
				}
				ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
				ch.Close()
			}()
		case "exec":
			l := binary.BigEndian.Uint32(r.Payload)
			cmd := exec.Command("bash", "-c", string(r.Payload[4:4+l]))
			r.Reply(true, nil)
			out, _ := cmd.CombinedOutput()
			ch.Write(out)
			ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
			ch.Close()
		default:
			if r.WantReply {
				r.Reply(true, nil)
			}
		}
	}
}
