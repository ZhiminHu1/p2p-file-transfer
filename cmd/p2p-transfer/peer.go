package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"tarun-kavipurapu/p2p-transfer/peer"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"

	"github.com/c-bata/go-prompt"
	"github.com/spf13/cobra"
)

var (
	peerAddr        string
	serverAddr      string
	fileToRegister  string
	fileToDownload  string
	peerInteractive bool
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

		if peerInteractive {
			fmt.Println("P2P Peer Node Interactive Shell")
			fmt.Println("Type 'help' for commands.")

			prompt.New(
				func(in string) { peerExecutor(in, p) }, // in代表用户终端的输入
				peerCompleter,                           // 命令自动补全的集合
				prompt.OptionPrefix("peer> "),           // 显示的前缀
				prompt.OptionTitle("P2P Peer Node"),
			).Run()
		} else {
			select {}
		}
	},
}

func peerExecutor(in string, p *peer.PeerServer) {
	in = strings.TrimSpace(in)
	blocks := strings.Fields(in)
	if len(blocks) == 0 {
		return
	}

	switch blocks[0] {
	case "exit", "quit":
		fmt.Println("Stopping peer...")
		p.Stop()
		os.Exit(0)
	case "status":
		fmt.Println(p.GetStatus())
	case "register":
		if len(blocks) < 2 {
			fmt.Println("Usage: register <file_path>")
			return
		}
		path := blocks[1]
		if err := p.RegisterFile(path); err != nil {
			fmt.Printf("Error registering file: %v\n", err)
		} else {
			fmt.Println("File registered successfully.")
		}
	case "download":
		if len(blocks) < 2 {
			fmt.Println("Usage: download <file_hash_id>")
			return
		}
		hash := blocks[1]
		if err := p.RequestChunkData(hash); err != nil {
			fmt.Printf("Error requesting download: %v\n", err)
		} else {
			fmt.Println("Download request sent.")
		}
	case "help":
		fmt.Println("Available commands:")
		fmt.Println("  status                 - Show peer status")
		fmt.Println("  register <path>        - Register and seed a local file")
		fmt.Println("  download <hash>        - Download a file by ID")
		fmt.Println("  exit                   - Stop peer and exit")
	default:
		fmt.Println("Unknown command: " + blocks[0])
	}
}

func peerCompleter(d prompt.Document) []prompt.Suggest {
	s := []prompt.Suggest{
		{Text: "status", Description: "Show peer status"},
		{Text: "register", Description: "Register a file"},
		{Text: "download", Description: "Download a file"},
		{Text: "exit", Description: "Exit the peer"},
		{Text: "help", Description: "Show help"},
	}
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

func init() {
	rootCmd.AddCommand(peerCmd)
	peerCmd.Flags().StringVarP(&peerAddr, "addr", "a", "127.0.0.1:8001", "Address for this peer to listen on")
	peerCmd.Flags().StringVarP(&serverAddr, "server", "s", "127.0.0.1:8000", "Address of the Central Server")
	peerCmd.Flags().StringVarP(&fileToRegister, "register", "r", "", "Path to a file to register/seed immediately")
	peerCmd.Flags().StringVarP(&fileToDownload, "download", "d", "", "File Hash ID to download immediately")
	peerCmd.Flags().BoolVarP(&peerInteractive, "interactive", "i", false, "Start in interactive mode")
}
