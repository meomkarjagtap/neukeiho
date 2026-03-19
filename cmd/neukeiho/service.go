package main

import (
	"fmt"
	"os"
	"os/exec"
)

const systemdService = `[Unit]
Description=NeuKeiho — Hybrid VM Monitoring & AI Alerting
After=network.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/neukeiho start
Restart=on-failure
RestartSec=5s
StandardOutput=append:/var/log/neukeiho/neukeiho.log
StandardError=append:/var/log/neukeiho/neukeiho.log

[Install]
WantedBy=multi-user.target
`

const servicePath = "/etc/systemd/system/neukeiho.service"

func runInstall() {
	fmt.Println("[neukeiho] installing systemd service...")

	// ensure dirs exist
	dirs := []string{
		"/etc/neukeiho",
		"/var/log/neukeiho",
		"/var/lib/neukeiho",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create %s: %v\n", d, err)
			os.Exit(1)
		}
	}

	// write service file
	if err := os.WriteFile(servicePath, []byte(systemdService), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write service file: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: run with sudo")
		os.Exit(1)
	}

	// run init if config doesn't exist yet
	if _, err := os.Stat("/etc/neukeiho/neukeiho.conf"); os.IsNotExist(err) {
		fmt.Println("[neukeiho] no config found — running init first...")
		runInit()
	}

	// systemctl daemon-reload
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "daemon-reload failed: %v\n", err)
		os.Exit(1)
	}

	// enable + start
	if err := exec.Command("systemctl", "enable", "neukeiho").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "systemctl enable failed: %v\n", err)
		os.Exit(1)
	}
	if err := exec.Command("systemctl", "start", "neukeiho").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "systemctl start failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ neukeiho service installed and started")
	fmt.Println()
	fmt.Println("   Useful commands:")
	fmt.Println("   sudo systemctl status neukeiho")
	fmt.Println("   sudo systemctl restart neukeiho")
	fmt.Println("   sudo journalctl -u neukeiho -f")
	fmt.Println("   sudo neukeiho uninstall")
}

func runUninstall() {
	fmt.Println("[neukeiho] removing systemd service...")

	cmds := [][]string{
		{"systemctl", "stop", "neukeiho"},
		{"systemctl", "disable", "neukeiho"},
	}
	for _, args := range cmds {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "%v failed: %v (continuing)\n", args, err)
		}
	}

	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "failed to remove service file: %v\n", err)
		os.Exit(1)
	}

	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "daemon-reload failed: %v\n", err)
	}

	fmt.Println("✅ neukeiho service removed")
	fmt.Println("   Config and data at /etc/neukeiho and /var/lib/neukeiho are preserved.")
	fmt.Println("   Remove manually if needed.")
}
