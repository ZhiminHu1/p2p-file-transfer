package main

import (
	"time"

	"tarun-kavipurapu/p2p-transfer/peer"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"

	"github.com/spf13/cobra"
)

var (
	peerAddr       string
	serverAddr     string
	fileToRegister string
	fileToDownload string
)

var peerCmd = &cobra.Command{
	Use:   "peer",
	Short: "Start a Peer Node",
	Run: func(cmd *cobra.Command, args []string) {
		logger.Sugar.Infof("Starting Peer Node on %s, connecting to Server %s", peerAddr, serverAddr)

		p := peer.NewPeerServer(peerAddr, serverAddr)

		// Start peer server in background
		go func() {
			if err := p.Start(); err != nil {
				logger.Sugar.Fatal("Error starting peer server: ", err)
			}
		}()

		// Wait for connection to be established
		// TODO: Implement a better readiness check
		time.Sleep(1 * time.Second)

		// Handle immediate actions
		if fileToRegister != "" {
			logger.Sugar.Infof("Auto-registering file: %s", fileToRegister)
			if err := p.RegisterFile(fileToRegister); err != nil {
				logger.Sugar.Errorf("Failed to register file: %v", err)
			}
		}

		if fileToDownload != "" {
			logger.Sugar.Infof("Auto-downloading file ID: %s", fileToDownload)
			if err := p.RequestChunkData(fileToDownload); err != nil {
				logger.Sugar.Errorf("Failed to request download: %v", err)
			}
		}

		// Block forever
		select {}
	},
}

func init() {
	rootCmd.AddCommand(peerCmd)
	peerCmd.Flags().StringVarP(&peerAddr, "addr", "a", "127.0.0.1:8001", "Address for this peer to listen on")
	peerCmd.Flags().StringVarP(&serverAddr, "server", "s", "127.0.0.1:8000", "Address of the Central Server")
	peerCmd.Flags().StringVarP(&fileToRegister, "register", "r", "", "Path to a file to register/seed immediately")
	peerCmd.Flags().StringVarP(&fileToDownload, "download", "d", "", "File Hash ID to download immediately")
}
