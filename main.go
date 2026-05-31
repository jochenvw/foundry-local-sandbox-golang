package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"jvw.com/flsandbox/app"
)

func main() {
	var (
		modelAlias   = flag.String("model", "phi-4-mini", "Model alias or ID")
		systemPrompt = flag.String("system", "You are a helpful AI assistant.", "System prompt")
		device       = flag.String("device", "auto", "Preferred device (auto|cpu|gpu|npu)")
		temperature  = flag.Float64("temp", 0.2, "Sampling temperature")
		ctxLimit     = flag.Int("ctx", 4096, "Context window size")
		locality     = flag.String("mode", "local", "Session mode label (local|remote)")
	)

	flag.Parse()

	if _, _, err := app.ParseDeviceChoice(*device); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	cfg := app.ModelConfig{
		Alias:        *modelAlias,
		DeviceChoice: *device,
		SystemPrompt: *systemPrompt,
		Temperature:  *temperature,
		CtxLimit:     *ctxLimit,
		Locality:     *locality,
		Remote:       *locality == "remote",
	}

	model := app.NewModel(cfg)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
