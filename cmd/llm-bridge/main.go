package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/anthropics/llm-bridge/internal/bridge"
	"github.com/anthropics/llm-bridge/internal/config"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "llm-bridge",
	Short: "Bridge Discord to Claude",
	Long:  "A bidirectional bridge connecting chat platforms to LLM CLIs.",
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the bridge server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			slog.Info("shutting down")
			cancel()
		}()

		b := bridge.New(cfg)
		slog.Info("starting bridge", "config", cfgFile)
		return b.Start(ctx)
	},
}

var addRepoCmd = &cobra.Command{
	Use:   "add-repo <name>",
	Short: "Add a repository to the configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		providerFlag, _ := cmd.Flags().GetString("provider")
		channelFlag, _ := cmd.Flags().GetString("channel")
		llmFlag, _ := cmd.Flags().GetString("llm")
		dirFlag, _ := cmd.Flags().GetString("dir")
		gitRootFlag, _ := cmd.Flags().GetString("git-root")
		branchFlag, _ := cmd.Flags().GetString("branch")

		repo := config.RepoConfig{
			Provider:   providerFlag,
			ChannelID:  channelFlag,
			LLM:        llmFlag,
			WorkingDir: dirFlag,
			GitRoot:    gitRootFlag,
			Branch:     branchFlag,
		}

		if err := config.AddRepo(cfgFile, name, repo); err != nil {
			return err
		}

		fmt.Printf("Added repo %q to %s\n", name, cfgFile)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "llm-bridge.yaml", "config file path")

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(addRepoCmd)

	addRepoCmd.Flags().String("provider", "discord", "Chat provider (discord)")
	addRepoCmd.Flags().String("channel", "", "Channel ID")
	addRepoCmd.Flags().String("llm", "claude", "LLM backend")
	addRepoCmd.Flags().String("dir", ".", "Working directory")
	addRepoCmd.Flags().String("git-root", "", "Git root directory (for worktree repos)")
	addRepoCmd.Flags().String("branch", "", "Branch name (informational)")
	_ = addRepoCmd.MarkFlagRequired("channel")
	_ = addRepoCmd.MarkFlagRequired("dir")
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
