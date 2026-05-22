// check_links.go — verifies every relative markdown link in the repo resolves.
//
// Run from the repo root:
//   go run tools/scripts/check_links.go .
//
// Returns non-zero if any link is broken. Used by CI (.github/workflows/ci.yml).
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	mdLink := regexp.MustCompile(`\]\(([^)]+)\)`)
	totalLinks := 0
	brokenLinks := 0

	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if strings.Contains(p, "/.git") || strings.Contains(p, "/node_modules") || strings.Contains(p, "/vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") {
			return nil
		}

		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			for _, m := range mdLink.FindAllStringSubmatch(line, -1) {
				target := m[1]
				if strings.HasPrefix(target, "http") ||
					strings.HasPrefix(target, "mailto:") ||
					strings.HasPrefix(target, "#") ||
					strings.HasPrefix(target, "/") {
					continue
				}
				totalLinks++
				clean := target
				if i := strings.IndexAny(clean, "#?"); i >= 0 {
					clean = clean[:i]
				}
				if clean == "" {
					continue
				}
				resolved := filepath.Join(filepath.Dir(p), clean)
				if _, err := os.Stat(resolved); err != nil {
					brokenLinks++
					rel, _ := filepath.Rel(root, p)
					fmt.Printf("BROKEN: %s:%d → %s\n", rel, lineNo, target)
				}
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	fmt.Printf("\nTotal relative links: %d, broken: %d\n", totalLinks, brokenLinks)
	if brokenLinks != 0 {
		os.Exit(1)
	}
}
