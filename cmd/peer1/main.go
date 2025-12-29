package main

import (
	"log"
	"os"
	"tarun-kavipurapu/p2p-transfer/peer"
	"tarun-kavipurapu/p2p-transfer/pkg"
	"time"
)

func main() {

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	opts := pkg.TransportOpts{
		ListenAddr: "127.0.0.1:8002",
		Decoder:    pkg.GOBDecoder{},
	}
	p := peer.NewPeerServer(opts, "127.0.0.1:8000")
	go func() {
		err := p.Start()
		if err != nil {
			log.Fatalln(err)
		}

	}()

	time.Sleep(1 * time.Second)

	err := p.RequestChunkData("56f89f1ae7bd3bba44d9c422cad2509ddd721406834f80181e59857f95ef784a")

	if err != nil {
		log.Println("Error in Chunk Request", err)
	}
	select {}
}
