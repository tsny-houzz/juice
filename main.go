package main

import (
	"bufio"
	"context"
	"encoding/json"
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
	"github.com/urfave/cli"
	appsV1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Options struct {
	Verbose     bool
	MaxPods     int
	LogLevel    string
	Grep        string
	PrintFull   bool
	Raw         bool
	Interactive bool
	PodName     string
	App         string
	Container   string
	Namespace   string
	Context     string
}

func main() {
	app := &cli.App{
		Name:  "log-tail",
		Usage: "Tail Kubernetes logs with optional JSON parsing & filters",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "v",
				Usage: "Verbose mode",
			},
			&cli.IntFlag{
				Name:  "max-pods",
				Usage: "Maximum number of pods to tail logs from",
				Value: 20,
			},
			&cli.StringFlag{
				Name:   "level",
				Usage:  "Only print logs with this level (case-insensitive)",
				EnvVar: "LOG_LEVEL",
			},
			&cli.StringFlag{
				Name:  "grep",
				Usage: "Only print logs matching this grep pattern",
			},
			&cli.BoolFlag{
				Name:  "full",
				Usage: "Print the full log struct",
			},
			&cli.BoolFlag{
				Name:  "raw",
				Usage: "Print raw log lines without any formatting",
			},
			&cli.BoolFlag{
				Name:  "i",
				Usage: "Interactive mode to select app label",
			},
			&cli.StringFlag{
				Name:  "pod",
				Usage: "If set, tail logs from this specific pod",
			},
			&cli.StringFlag{
				Name:  "app",
				Usage: "App label to match",
				Value: "jukwaa-main",
			},
			&cli.StringFlag{
				Name:  "c",
				Usage: "Container name to tail (empty = first container)",
			},
			&cli.StringFlag{
				Name:  "ctx",
				Usage: "Kubernetes context",
			},
			&cli.StringFlag{
				Name:  "n",
				Usage: "Kubernetes namespace",
			},
		},
		Action: func(c *cli.Context) error {
			opts := Options{
				Verbose:     c.Bool("v"),
				MaxPods:     c.Int("max-pods"),
				LogLevel:    c.String("level"),
				Grep:        c.String("grep"),
				PrintFull:   c.Bool("full"),
				Raw:         c.Bool("raw"),
				Interactive: c.Bool("i"),
				PodName:     c.String("pod"),
				App:         c.String("app"),
				Container:   c.String("c"),
				Namespace:   c.String("n"),
				Context:     c.String("ctx"),
			}

			return run(opts)
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(opts Options) error {
	if opts.Interactive {
		c, _ := InferClient()

		if opts.Namespace == "" {
			nsList, err := c.Kube.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to list namespaces: %v\n", err)
				os.Exit(1)
			}

			namespaces := []string{}
			for _, ns := range nsList.Items {
				namespaces = append(namespaces, ns.Name)
			}
			sort.Strings(namespaces)

			p := promptui.Select{
				Label: "Select Namespace",
				Items: namespaces,
				Size:  20,
			}
			_, res, err := p.Run()
			if err != nil {
				fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
				os.Exit(1)
			}
			opts.Namespace = res
		}

		dps, err := c.Kube.AppsV1().Deployments(opts.Namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list deployments in namespace %s: %v\n", opts.Namespace, err)
			os.Exit(1)
		}

		// Filter for dps with both "app" and "component" labels
		dpMap := map[string]appsV1.Deployment{}
		for _, dp := range dps.Items {
			if *dp.Spec.Replicas == 0 {
				continue
			}
			if dp.Labels["component"] != "" && dp.Labels["app"] != "" {
				dpMap[dp.GetName()] = dp
			}
		}

		dpOpts := []string{}
		dpNames := []string{}
		for _, dp := range dpMap {
			dpOpts = append(dpOpts, fmt.Sprintf("%v (%v)", dp.Labels["app"], dp.CreationTimestamp.Local().Format("2006-01-02 15:04:05")))
			dpNames = append(dpNames, dp.GetName())
		}

		sort.Strings(dpOpts)

		p := promptui.Select{
			Label: "Select Deployment",
			Items: dpOpts,
			Size:  20,
		}
		i, _, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
			os.Exit(1)
		}

		containerNames := []string{}
		dp := dpMap[dpNames[i]]
		opts.App = dp.Labels["app"]

		for _, c := range dp.Spec.Template.Spec.Containers {
			containerNames = append(containerNames, c.Name)
		}

		p = promptui.Select{
			Label: "Select Container",
			Items: containerNames,
			Size:  20,
		}
		_, opts.Container, err = p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
			os.Exit(1)
		}
	}

	args := []string{
		"logs", "-f",
		"-n", opts.Namespace,
		"--ignore-errors",
		"--tail", "100",
	}

	if opts.Context != "" {
		args = append(args, "--context", opts.Context)
	}
	if opts.MaxPods > 0 {
		args = append(args, fmt.Sprintf("--max-log-requests=%d", opts.MaxPods))
	}
	if opts.PodName != "" {
		args = append(args, opts.PodName)
	} else {
		args = append(args, "-l", fmt.Sprintf("app=%s", opts.App))
	}

	if opts.Container != "" {
		args = append(args, "-c", opts.Container)
	} else {
		args = append(args, "--all-containers")
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

		// Skip common health check logs
		if strings.Contains(line, "readiness") || strings.Contains(line, "liveness") {
			continue
		}

		if opts.Grep != "" && !strings.Contains(line, opts.Grep) {
			continue
		}

		if opts.Raw {
			fmt.Println(line)
			continue
		}

		var rec LogRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			// Print anyway
			fmt.Println("non-json:", line)
			if opts.Verbose {
				fmt.Println("unmarshal error:", err.Error())
			}
			continue
		}

		// If log-level flag is set, only print matching levels
		if opts.LogLevel != "" && !strings.EqualFold(rec.Level, opts.LogLevel) {
			continue
		}

		if opts.PrintFull {
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
			msg := fmt.Sprintf("[%v] ", coloredLevel)
			if rec.LevelMeta.Request.URL != "" {
				msg += fmt.Sprintf("[%v]", rec.LevelMeta.Request.URL)
			}
			if rec.LevelMeta.User.Name != "" {
				msg += fmt.Sprintf(" [%v]", rec.LevelMeta.User.Name)
			}
			msg += fmt.Sprintf(" %v", rec.Message)
			fmt.Println(msg)
			if rec.Level == "error" && rec.Stack != "" {
				// Pretty print the stack if it's an error log with a stack trace.
				fmt.Println(rec.Stack)
			}
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

	return cmd.Wait()
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
	status := nonEmpty(md.Status, "-")
	url := nonEmpty(md.URL, "/")
	rt := nonEmpty(md.ResponseTimeMS, "?") + "ms"
	ip := nonEmpty(md.ClientIP, md.RemoteAddr)

	return fmt.Sprintf("%s %s %s | %s %s | %s | %s",
		method,
		statusToColor(status),
		url,
		md.Domain,
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

func statusToColor(status string) string {
	if strings.HasPrefix(status, "2") {
		return "\033[32m" + status + "\033[0m" // green
	} else if strings.HasPrefix(status, "3") {
		return "\033[36m" + status + "\033[0m" // cyan
	} else if strings.HasPrefix(status, "4") {
		return "\033[33m" + status + "\033[0m" // yellow
	} else if strings.HasPrefix(status, "5") {
		return "\033[31m" + status + "\033[0m" // red
	}
	return status
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
