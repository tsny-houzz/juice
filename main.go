package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/manifoldco/promptui"
	appsV1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	var logLevelFlag string
	var printFullStruct bool
	var raw bool
	var interactiveMode bool
	var podName string

	flag.StringVar(&logLevelFlag, "log-level", "", "Only print logs with this level (case-insensitive)")
	flag.BoolVar(&printFullStruct, "full", false, "Print the full log struct")
	flag.BoolVar(&raw, "raw", false, "Print raw log lines without any formatting")
	flag.BoolVar(&interactiveMode, "i", false, "Interactive mode to select app label")
	flag.StringVar(&podName, "pod", "", "If set, tail logs from this specific pod")
	flag.Parse()

	app := envOr("APP", "jukwaa-main")
	container := envOr("CONTAINER_NAME", "jukwaa")
	namespace := envOr("NAMESPACE", "stghouzz")

	if interactiveMode {
		c, _ := InferClient()
		dps, err := c.Kube.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list deployments in namespace %s: %v\n", namespace, err)
			os.Exit(1)
		}

		// Filter for dps with both "app" and "component" labels
		newDps := map[string]appsV1.Deployment{}
		for _, dp := range dps.Items {
			if *dp.Spec.Replicas == 0 {
				continue
			}
			if dp.Labels["component"] != "" && dp.Labels["app"] != "" {
				newDps[dp.GetName()] = dp
			}
		}

		dpNames := []string{}
		for _, dp := range newDps {
			dpNames = append(dpNames, dp.GetName())
		}

		sort.Strings(dpNames)

		p := promptui.Select{
			Label: "Select Deployment",
			Items: dpNames,
			Size:  20,
		}
		_, res, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
			os.Exit(1)
		}

		containerNames := []string{}
		dp := newDps[res]
		app = dp.Labels["app"]

		for _, c := range dp.Spec.Template.Spec.Containers {
			containerNames = append(containerNames, c.Name)
		}

		p = promptui.Select{
			Label: "Select Container",
			Items: containerNames,
			Size:  20,
		}
		_, container, err = p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
			os.Exit(1)
		}
	}

	args := []string{
		"logs", "-f",
		"-n", namespace,
		"--ignore-errors",
	}
	if podName != "" {
		args = append(args, podName)
	} else {
		args = append(args, "-l", fmt.Sprintf("app=%s", app))
	}

	if container != "" {
		args = append(args, "-c", container)
	}

	ctx, cancel := signalContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	fmt.Println(">", cmd.String())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get stdout: %v\n", err)
		os.Exit(1)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get stderr: %v\n", err)
		os.Exit(1)
	}
	go io.Copy(os.Stderr, stderr)

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start kubectl: %v\n", err)
		os.Exit(1)
	}

	sc := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 256*1024)
	sc.Buffer(buf, 2*1024*1024)

	for sc.Scan() {
		line := sc.Text()
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}

		if raw {
			fmt.Println(line)
			continue
		}

		// Skip common health check logs
		if strings.Contains(line, "readiness") || strings.Contains(line, "liveness") {
			continue
		}

		var rec LogRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			// Print anyway
			fmt.Println(line)
			continue
		}

		// If log-level flag is set, only print matching levels
		if logLevelFlag != "" && !strings.EqualFold(rec.Level, logLevelFlag) {
			continue
		}

		if printFullStruct {
			// Pretty print the full struct
			out, err := json.MarshalIndent(rec, "", "  ")
			if err != nil {
				fmt.Println(line)
			} else {
				fmt.Println(string(out))
			}
			continue
		}

		// If Message is non-empty, print just the message.
		if strings.TrimSpace(rec.Message) != "" {
			coloredLevel := colorizeLevel(rec.Level)
			msg := fmt.Sprintf("[%v] %v", coloredLevel, rec.Message)
			if rec.LevelMeta.Request.URL != "" {
				msg = fmt.Sprintf("[%v] [%v] %v", coloredLevel, rec.LevelMeta.Request.URL, rec.Message)
			}
			fmt.Println(msg)
			continue
		}

		// Otherwise pretty print a curl-like block from the access metadata.
		m := shortCurlish(rec)
		if m != "" {
			fmt.Println(m)
		}
	}

	if err := sc.Err(); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
	}

	_ = cmd.Wait()
}

// -------- Helpers --------

// getDeployments queries the given namespace for deployments using kubectl and returns the raw JSON output.
func getDeployments(namespace string) ([]byte, error) {
	cmd := exec.Command("kubectl", "get", "deployments", "-n", namespace, "-o", "json")
	return cmd.Output()
}

// getAppLabelsFromDeployments parses the deployments JSON and returns a slice of unique 'app' label values.
func getAppLabelsFromDeployments(deploymentsJSON []byte) ([]string, error) {
	var parsed struct {
		Items []struct {
			Metadata struct {
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(deploymentsJSON, &parsed); err != nil {
		return nil, err
	}
	appSet := make(map[string]struct{})
	for _, item := range parsed.Items {
		if app, ok := item.Metadata.Labels["app"]; ok && app != "" {
			appSet[app] = struct{}{}
		}
	}
	var apps []string
	for app := range appSet {
		apps = append(apps, app)
	}
	return apps, nil
}

func signalContext(parent context.Context, sig ...os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sig...)
	go func() {
		defer signal.Stop(ch)
		select {
		case <-ch:
			cancel()
			time.Sleep(100 * time.Millisecond)
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func shortCurlish(rec LogRecord) string {
	md := rec.Metadata

	// Pick a handful of useful fields.
	method := nonEmpty(md.Method, "GET")
	url := nonEmpty(md.URL, "/")
	status := nonEmpty(md.Status, "-")
	rt := nonEmpty(md.ResponseTimeMS, "?") + "ms"
	ip := nonEmpty(md.ClientIP, md.RemoteAddr)

	return fmt.Sprintf("%s %s %s | %s %s | %s | %s",
		method,
		url,
		md.Domain,
		status,
		rt,
		ip,
		md.RequestID,
	)
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// colorizeLevel returns the log level string wrapped in ANSI color codes.
func colorizeLevel(level string) string {
	switch strings.ToLower(level) {
	case "error":
		return "\033[31m" + level + "\033[0m" // red
	case "warn", "warning":
		return "\033[33m" + level + "\033[0m" // yellow
	case "info":
		return "\033[36m" + level + "\033[0m" // cyan
	case "debug":
		return "\033[35m" + level + "\033[0m" // magenta
	default:
		return level
	}
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
