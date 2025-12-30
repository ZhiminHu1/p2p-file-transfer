package main

import (
	"sync"
	"tarun-kavipurapu/p2p-transfer/peer"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"

	"time"
)

func main() {

	//this is the ip adress which the peer is listening
	p := peer.NewPeerServer("127.0.0.1:8001", "127.0.0.1:8000")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.Start()
		if err != nil {
			logger.Sugar.Fatal(err)
		}

	}()

	time.Sleep(1 * time.Second)
	//err := p.RegisterFile("C:\\Users\\13237\\Desktop\\githubproject\\p2p-file-transfer\\TestingFiles\\test_image.tif")
	err := p.RegisterFile("D:\\video\\20251212_125553.mp4")
	// err := p.RegisterFile("D:\\Devlopement\\go-network-Stream\\TestingFiles\\test_image.tif")
	if err != nil {
		logger.Sugar.Error("Error Register peer", err)
	}

	wg.Wait()
	// err = p.RequestChunkData("017934bd678d02194f615f9e27cab2a72839fc28653daaf81ac7933f2945467d")

}
