package main

import (
	"log"
	"os"
	"tarun-kavipurapu/p2p-transfer/peer"

	"time"
)

func main() {

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	p := peer.NewPeerServer("127.0.0.1:8002", "127.0.0.1:8000")
	go func() {
		err := p.Start()
		if err != nil {
			log.Fatalln(err)
		}

	}()

	time.Sleep(1 * time.Second)

	err := p.RequestChunkData("017934bd678d02194f615f9e27cab2a72839fc28653daaf81ac7933f2945467d")

	if err != nil {
		log.Println("Error in Chunk Request", err)
	}
	select {}
}
