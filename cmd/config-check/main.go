package main

import (
	"flag"
	"fmt"
	"os"

	"modern-dhcp/internal/config"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")
	scope := flag.String("scope", "auth", "Validation scope (auth|all)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	switch *scope {
	case "auth", "all":
		if err := cfg.ValidateAuth(); err != nil {
			fmt.Fprintf(os.Stderr, "config validation failed: %v\n", err)
			os.Exit(2)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported scope: %s\n", *scope)
		os.Exit(1)
	}

	fmt.Println("config validation passed")
}
