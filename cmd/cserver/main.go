package main

import (
	centralserver "tarun-kavipurapu/p2p-transfer/central-server"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"
)

func main() {

	opts := "127.0.0.1:8000"
	server := centralserver.NewCentralServer(opts)

	err := server.Start()

	if err != nil {
		logger.Sugar.Error("Error Starting cserver ", err)
	}

}
