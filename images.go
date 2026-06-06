package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type ImageDef struct {
	Name    string            `json:"name"`
	URL     string            `json:"url,omitempty"`
	Path    string            `json:"path"`
	Env     map[string]string `json:"env,omitempty"`
	Workdir string            `json:"workdir,omitempty"`
}

var predefinedImages = map[string]ImageDef{
	"node:20": {
		Name: "node:20",
		Path: "/usr/local/node/bin:/usr/local/bin:/usr/bin:/bin",
		Env: map[string]string{
			"NODE_ENV": "production",
		},
		Workdir: "/home",
	},
	"python:3": {
		Name: "python:3",
		Path: "/usr/local/python/bin:/usr/local/bin:/usr/bin:/bin",
		Env: map[string]string{
			"PYTHONDONTWRITEBYTECODE": "1",
			"PYTHONUNBUFFERED":        "1",
		},
		Workdir: "/home",
	},
	"go:latest": {
		Name: "go:latest",
		Path: "/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin",
		Env: map[string]string{
			"GOPATH":     "/home/go",
			"GO111MODULE": "on",
		},
		Workdir: "/home",
	},
	"alpine:latest": {
		Name:    "alpine:latest",
		Path:    "/usr/local/bin:/usr/bin:/bin",
		Env:     map[string]string{},
		Workdir: "/home",
	},
}

func imagesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".flect", "images")
}

func runPull(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "%s usage: flect pull <name> [<url>]\n", fail(""))
		fmt.Fprintln(os.Stderr)
		fmt.Println("Predefined images:")
		for name := range predefinedImages {
			fmt.Printf("  %s\n", name)
		}
		os.Exit(1)
	}

	name := args[0]
	url := ""
	if len(args) > 1 {
		url = args[1]
	}

	baseDir := imagesDir()
	if baseDir == "" {
		fmt.Fprintf(os.Stderr, "%s cannot determine home directory\n", fail(""))
		os.Exit(1)
	}

	imgDir := filepath.Join(baseDir, name)
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "%s creating image directory: %v\n", fail(""), err)
		os.Exit(1)
	}

	var imgDef ImageDef

	if predefined, ok := predefinedImages[name]; ok {
		imgDef = predefined
		imgDef.Name = name
		fmt.Printf("  %s preparing predefined image %s\n", cyan("●"), cyanBold(name))
	} else if url != "" {
		imgDef = ImageDef{
			Name:    name,
			Path:    "/usr/local/bin:/usr/bin:/bin",
			Env:     make(map[string]string),
			Workdir: "/home",
		}
		fmt.Printf("  %s downloading %s...\n", cyan("●"), url)
		if err := downloadAndExtract(url, imgDir); err != nil {
			fmt.Fprintf(os.Stderr, "%s download failed: %v\n", fail(""), err)
			os.Exit(1)
		}
		fmt.Printf("  %s extracted to %s\n", green(check), imgDir)
	} else {
		fmt.Fprintf(os.Stderr, "%s unknown image %q\n", fail(""), name)
		fmt.Fprintln(os.Stderr)
		fmt.Println("Available predefined images:")
		for n := range predefinedImages {
			fmt.Printf("  %s\n", n)
		}
		os.Exit(1)
	}

	data, err := json.MarshalIndent(imgDef, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s encoding image: %v\n", fail(""), err)
		os.Exit(1)
	}

	envPath := filepath.Join(imgDir, "env.json")
	if err := os.WriteFile(envPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "%s writing env.json: %v\n", fail(""), err)
		os.Exit(1)
	}

	fmt.Printf("  %s image %s pulled\n", green(check), cyanBold(name))
}

func downloadAndExtract(url, destDir string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpFile := filepath.Join(destDir, "image.tar.gz")
	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpFile)
		return err
	}

	if err := extractTarGz(tmpFile, destDir); err != nil {
		os.Remove(tmpFile)
		return err
	}

	os.Remove(tmpFile)
	return nil
}

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Name == "" {
			continue
		}

		target := filepath.Join(dest, "rootfs", header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			of, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, err = io.Copy(of, tr)
			of.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func runImages() {
	dir := imagesDir()
	if dir == "" {
		fmt.Fprintf(os.Stderr, "%s cannot determine home directory\n", fail(""))
		os.Exit(1)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println(warnMsg("no cached images"))
			fmt.Println("  try: flect pull node:20")
			return
		}
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("reading images:"), err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println(warnMsg("no cached images"))
		fmt.Println("  try: flect pull node:20")
		return
	}

	fmt.Printf("  %s cached images:\n\n", cyan("●"))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		envPath := filepath.Join(dir, name, "env.json")
		if _, err := os.Stat(envPath); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(envPath)
		if err != nil {
			continue
		}
		var img ImageDef
		if err := json.Unmarshal(data, &img); err != nil {
			continue
		}

		rootfs := filepath.Join(dir, name, "rootfs")
		rootfsInfo, _ := os.Stat(rootfs)
		size := "-"
		if rootfsInfo != nil && rootfsInfo.IsDir() {
			size = dirSize(rootfs)
		}

		if _, predefined := predefinedImages[name]; predefined {
			fmt.Printf("  %s %s (predefined) %s\n", green(check), cyanBold(name), size)
		} else {
			fmt.Printf("  %s %s %s\n", green(check), cyanBold(name), size)
		}

		if img.Path != "" {
			paths := strings.Split(img.Path, ":")
			for _, p := range paths {
				fmt.Printf("    path: %s\n", p)
			}
		}
		for k, v := range img.Env {
			fmt.Printf("    %s=%s\n", k, v)
		}
		fmt.Println()
	}
}

func dirSize(path string) string {
	var total int64
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	if total < 1024 {
		return fmt.Sprintf("%dB", total)
	}
	if total < 1024*1024 {
		return fmt.Sprintf("%dKB", total/1024)
	}
	return fmt.Sprintf("%dMB", total/(1024*1024))
}
