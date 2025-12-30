package main

import (
	"fmt"
	centralserver "tarun-kavipurapu/p2p-transfer/central-server"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"

	"github.com/spf13/cobra"
)

var (
	serverPort int
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Central Server",
	Run: func(cmd *cobra.Command, args []string) {
		addr := fmt.Sprintf("127.0.0.1:%d", serverPort)
		logger.Sugar.Infof("Starting Central Server on %s", addr)

		server := centralserver.NewCentralServer(addr)

		if err := server.Start(); err != nil {
			logger.Sugar.Error("Error Starting cserver ", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 8000, "Port to listen on")
}
