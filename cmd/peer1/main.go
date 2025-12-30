package main

import (
	"tarun-kavipurapu/p2p-transfer/peer"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"

	"time"
)

func main() {

	p := peer.NewPeerServer("127.0.0.1:8002", "127.0.0.1:8000")
	go func() {
		err := p.Start()
		if err != nil {
			logger.Sugar.Fatal(err)
		}

	}()

	time.Sleep(1 * time.Second)

	err := p.RequestChunkData("56f89f1ae7bd3bba44d9c422cad2509ddd721406834f80181e59857f95ef784a")

	if err != nil {
		logger.Sugar.Error("Error in Chunk Request", err)
	}
	select {}
}
