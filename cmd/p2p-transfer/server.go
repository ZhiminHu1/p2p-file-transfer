package main

import (
	"fmt"
	"os"
	"strings"

	centralserver "tarun-kavipurapu/p2p-transfer/central-server"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"

	"github.com/c-bata/go-prompt"
	"github.com/spf13/cobra"
)

var (
	centralServerAddr string
	interactive       bool
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Central Server",
	Run: func(cmd *cobra.Command, args []string) {
		logger.Sugar.Infof("Starting Central Server on %s", centralServerAddr)

		server := centralserver.NewCentralServer(centralServerAddr)

		if interactive {
			// Run server in background
			go func() {
				if err := server.Start(); err != nil {
					logger.Sugar.Error("Error Starting cserver ", err)
					os.Exit(1)
				}
			}()

			fmt.Println("P2P Central Server Interactive Shell")
			fmt.Println("Type 'help' for commands.")

			p := prompt.New(
				func(in string) { serverExecutor(in, server) },
				serverCompleter,
				prompt.OptionPrefix("cserver> "),
				prompt.OptionTitle("P2P Central Server"),
			)
			p.Run()
		} else {
			// Run server in foreground
			if err := server.Start(); err != nil {
				logger.Sugar.Error("Error Starting cserver ", err)
			}
		}
	},
}

func serverExecutor(in string, server *centralserver.CentralServer) {
	in = strings.TrimSpace(in)
	blocks := strings.Fields(in)
	if len(blocks) == 0 {
		return
	}

	switch blocks[0] {
	case "exit", "quit":
		fmt.Println("Stopping server...")
		server.Stop()
		os.Exit(0)
	case "status":
		fmt.Println(server.GetStatus())
	case "list":
		if len(blocks) > 1 && blocks[1] == "peers" {
			peers := server.GetPeersList()
			if len(peers) == 0 {
				fmt.Println("No peers connected.")
			} else {
				fmt.Println("Connected Peers:")
				for _, p := range peers {
					fmt.Println("- " + p)
				}
			}
		} else {
			fmt.Println("Usage: list peers")
		}
	case "help":
		fmt.Println("Available commands:")
		fmt.Println("  status       - Show server status")
		fmt.Println("  list peers   - List connected peers")
		fmt.Println("  exit         - Stop server and exit")
	default:
		fmt.Println("Unknown command: " + blocks[0])
	}
}

func serverCompleter(d prompt.Document) []prompt.Suggest {
	s := []prompt.Suggest{
		{Text: "status", Description: "Show server status and stats"},
		{Text: "list peers", Description: "List all connected peer addresses"},
		{Text: "exit", Description: "Exit the server"},
		{Text: "help", Description: "Show help"},
	}
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVarP(&centralServerAddr, "addr", "a", "0.0.0.0:8000", "Central server addr to listen on")
	serverCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Start in interactive mode")
}
