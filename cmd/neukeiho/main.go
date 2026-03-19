package main

import (
	"fmt"
	"os"
)

const (
	defaultConf = "/etc/neukeiho/neukeiho.conf"
	defaultTOML = "/etc/neukeiho/neukeiho.toml"
	version     = "v0.2.0"
	githubRepo  = "meomkarjagtap/neukeiho"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		runInit()
	case "install":
		runInstall()
	case "uninstall":
		runUninstall()
	case "start":
		runStart()
	case "deploy":
		runDeploy()
	case "status":
		runStatus()
	case "ask":
		runAsk()
	case "test-alert":
		runTestAlert()
	case "update":
		runUpdate()
	case "version":
		fmt.Println("neukeiho " + version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`NeuKeiho (警報) — Hybrid VM Monitoring & AI Alerting ` + version + `

Usage:
  neukeiho init          Generate config files interactively
  neukeiho install       Install and start as a systemd service
  neukeiho uninstall     Remove the systemd service
  neukeiho start         Start controller manually (foreground)
  neukeiho deploy        Deploy agents to nodes via Ansible
  neukeiho status        Live view of all node metrics
  neukeiho ask "<q>"     Ask Ollama about your infra
  neukeiho test-alert    Fire a test alert to all configured channels
  neukeiho update        Update to the latest version from GitHub
  neukeiho version       Print version`)
}
