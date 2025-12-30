package main

import (
	"os"

	"tarun-kavipurapu/p2p-transfer/pkg/logger"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "p2p-transfer",
	Short: "P2P File Transfer System",
	Long:  `A robust Peer-to-Peer file transfer system with a central discovery server.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Sugar.Error(err)
		os.Exit(1)
	}
}
