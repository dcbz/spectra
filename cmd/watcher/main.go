package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"watcher/internal/config"
	"watcher/internal/rules"
	"watcher/internal/runtime"
	"watcher/internal/tui"
)

func main() {
	filesFlag := flag.String("files", "/var/log/auth.log", "Comma separated list of files to watch")
	configFlag := flag.String("config", "configs/example.rules.yaml", "Rule configuration file path")
	themeFlag := flag.String("theme", "vapor", "Theme name (vapor|midnight|dusk)")
	scrollbackFlag := flag.Int("scrollback", 800, "Maximum number of lines to retain in memory")
	showAllFlag := flag.Bool("show-all", false, "Render every log line (default highlights only matched events)")
	minSeverityFlag := flag.String("min-severity", "medium", "Lowest severity to show (critical|high|medium|low|normal)")
	flag.Parse()

	files := splitFiles(*filesFlag)
	if len(files) == 0 {
		log.Fatal("no files supplied via --files")
	}

	ctx, cancel := signalContext()
	defer cancel()

	ruleSet, err := rules.LoadFromFile(*configFlag)
	if err != nil {
		log.Fatalf("load rules: %v", err)
	}

	minSeverity, err := rules.ParseSeverity(*minSeverityFlag)
	if err != nil {
		log.Fatalf("min severity: %v", err)
	}

	ctrl := runtime.NewController(ctx, ruleSet, *showAllFlag, minSeverity)
	if err := ctrl.Apply(runtime.Selection{Files: files}); err != nil {
		log.Fatalf("start tailing: %v", err)
	}

	presets := config.BuildLogPresets(files)
	ruleGroups := runtime.BuildRuleGroups(ruleSet)

	model := tui.NewModel(tui.ModelConfig{
		Events:      ctrl.Events(),
		ThemeName:   *themeFlag,
		Scrollback:  *scrollbackFlag,
		Files:       files,
		ShowAll:     *showAllFlag,
		MinSeverity: minSeverity,
		Controller:  ctrl,
		Presets:     presets,
		RuleGroups:  ruleGroups,
	})

	if err := tea.NewProgram(model, tea.WithAltScreen()).Start(); err != nil {
		log.Fatal(err)
	}
}

func splitFiles(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 4)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer signal.Stop(c)
		select {
		case <-c:
			fmt.Println("\nshutting down...")
			cancel()
		case <-ctx.Done():
		}
		time.Sleep(100 * time.Millisecond)
	}()
	return ctx, cancel
}
