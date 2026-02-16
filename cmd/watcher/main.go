package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	goruntime "runtime"
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
	defaultFiles := "/var/log/auth.log"
	defaultConfig := "configs/example.rules.yaml"
	if goruntime.GOOS == "darwin" {
		defaultFiles = "/var/log/system.log"
		defaultConfig = "configs/macos.rules.yaml"
	}

	filesFlag := flag.String("files", defaultFiles, "Comma separated list of files to watch")
	configFlag := flag.String("config", defaultConfig, "Rule configuration file path")
	themeFlag := flag.String("theme", "vapor", "Theme name (vapor|midnight|dusk)")
	scrollbackFlag := flag.Int("scrollback", 800, "Maximum number of lines to retain in memory")
	showAllFlag := flag.Bool("show-all", false, "Render every log line (default highlights only matched events)")
	minSeverityFlag := flag.String("min-severity", "medium", "Lowest severity to show (critical|high|medium|low|normal)")
	macosFlag := flag.Bool("macos", false, "Use macOS unified logging (auto-streams log show)")
	flag.Parse()

	if *macosFlag {
		if goruntime.GOOS != "darwin" {
			log.Fatal("--macos flag is only supported on macOS")
		}
		runMacOSMode(*configFlag, *themeFlag, *scrollbackFlag, *showAllFlag, *minSeverityFlag)
		return
	}

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

	if err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Start(); err != nil {
		log.Fatal(err)
	}
}

func runMacOSMode(configPath, theme string, scrollback int, showAll bool, minSeverityStr string) {
	tmpFile, err := os.CreateTemp("", "spectra-macos-*.log")
	if err != nil {
		log.Fatalf("create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	ctx, cancel := signalContext()
	defer cancel()

	logCmd := exec.CommandContext(ctx, "log", "stream", "--style", "syslog", "--level", "info")
	logOut, err := logCmd.StdoutPipe()
	if err != nil {
		log.Fatalf("create log pipe: %v", err)
	}
	logCmd.Stderr = os.Stderr
	if err := logCmd.Start(); err != nil {
		log.Fatalf("start log stream: %v", err)
	}

	fmt.Println("Starting macOS unified log stream...")
	fmt.Printf("Streaming to: %s\n", tmpPath)
	fmt.Println("Loading rules and starting TUI...")
	fmt.Println()

	go func() {
		f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("open temp file: %v", err)
			return
		}
		defer f.Close()
		io.Copy(f, logOut)
	}()

	time.Sleep(500 * time.Millisecond)

	ruleSet, err := rules.LoadFromFile(configPath)
	if err != nil {
		log.Fatalf("load rules: %v", err)
	}

	minSeverity, err := rules.ParseSeverity(minSeverityStr)
	if err != nil {
		log.Fatalf("min severity: %v", err)
	}

	ctrl := runtime.NewController(ctx, ruleSet, showAll, minSeverity)
	if err := ctrl.Apply(runtime.Selection{Files: []string{tmpPath}}); err != nil {
		log.Fatalf("start tailing: %v", err)
	}

	presets := config.BuildLogPresets([]string{tmpPath})
	ruleGroups := runtime.BuildRuleGroups(ruleSet)

	model := tui.NewModel(tui.ModelConfig{
		Events:      ctrl.Events(),
		ThemeName:   theme,
		Scrollback:  scrollback,
		Files:       []string{"macOS Unified Log"},
		ShowAll:     showAll,
		MinSeverity: minSeverity,
		Controller:  ctrl,
		Presets:     presets,
		RuleGroups:  ruleGroups,
	})

	if err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Start(); err != nil {
		log.Fatal(err)
	}

	if logCmd.Process != nil {
		logCmd.Process.Kill()
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
