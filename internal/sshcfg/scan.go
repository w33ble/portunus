package sshcfg

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxIncludeDepth = 16

// DefaultConfigPath is the user's ssh client config, ~/.ssh/config.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("sshcfg home dir: %w", err)
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// ListHosts scans configPath (following Include) for concrete Host aliases.
// It understands only Host and Include; everything else, Match included, is ignored.
// A missing file yields an empty list. Duplicate aliases keep the first occurrence.
func ListHosts(configPath string) ([]Host, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("sshcfg home dir: %w", err)
	}
	return listHosts(configPath, home)
}

func listHosts(configPath, home string) ([]Host, error) {
	s := &scanner{home: home, seen: map[string]bool{}, active: map[string]bool{}}
	if err := s.file(configPath, 0, true); err != nil {
		return nil, err
	}
	if s.hosts == nil {
		s.hosts = []Host{}
	}
	return s.hosts, nil
}

type scanner struct {
	home   string
	hosts  []Host
	seen   map[string]bool // aliases
	active map[string]bool // files on the current include stack
}

// file scans one file; only the top-level file may error on read problems.
func (s *scanner) file(path string, depth int, top bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if s.active[abs] {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || !top {
			return nil
		}
		return fmt.Errorf("sshcfg open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	s.active[abs] = true
	defer delete(s.active, abs)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 1; sc.Scan(); line++ {
		kw, args := splitLine(sc.Text())
		switch kw {
		case "host":
			s.addHosts(path, line, args)
		case "include":
			if depth >= maxIncludeDepth {
				continue
			}
			if err := s.include(args, depth); err != nil {
				return err
			}
		}
	}
	if err := sc.Err(); err != nil && top {
		return fmt.Errorf("sshcfg read %s: %w", path, err)
	}
	return nil
}

func (s *scanner) addHosts(path string, line int, args []string) {
	for _, a := range args {
		if strings.ContainsAny(a, "*?!") || s.seen[a] {
			continue
		}
		s.seen[a] = true
		s.hosts = append(s.hosts, Host{Alias: a, Source: fmt.Sprintf("%s:%d", path, line)})
	}
}

func (s *scanner) include(args []string, depth int) error {
	for _, a := range args {
		for _, m := range s.expandInclude(a) {
			if err := s.file(m, depth+1, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *scanner) expandInclude(pat string) []string {
	switch {
	case pat == "~":
		pat = s.home
	case strings.HasPrefix(pat, "~/"):
		pat = filepath.Join(s.home, pat[2:])
	case !filepath.IsAbs(pat):
		pat = filepath.Join(s.home, ".ssh", pat)
	}
	m, err := filepath.Glob(pat)
	if err != nil {
		return nil
	}
	return m
}

// splitLine returns the lowercased keyword and unquoted arguments of a config line.
func splitLine(line string) (string, []string) {
	toks := tokenize(line)
	if len(toks) == 0 {
		return "", nil
	}
	kw := toks[0]
	// "Keyword=value" and "Keyword = value" forms
	if i := strings.IndexByte(kw, '='); i >= 0 {
		rest := kw[i+1:]
		kw = kw[:i]
		toks = toks[1:]
		if rest != "" {
			toks = append([]string{rest}, toks...)
		}
	} else {
		toks = toks[1:]
		if len(toks) > 0 && toks[0] == "=" {
			toks = toks[1:]
		} else if len(toks) > 0 && strings.HasPrefix(toks[0], "=") {
			toks[0] = toks[0][1:]
		}
	}
	return strings.ToLower(kw), toks
}

// tokenize splits on whitespace honoring double quotes; '#' outside quotes starts a comment.
func tokenize(line string) []string {
	var out []string
	var cur strings.Builder
	inTok, inQuote := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inQuote:
			if c == '"' {
				inQuote = false
			} else {
				cur.WriteByte(c)
			}
		case c == '"':
			inQuote, inTok = true, true
		case c == '#':
			i = len(line)
		case c == ' ' || c == '\t' || c == '\r':
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteByte(c)
			inTok = true
		}
	}
	if inTok {
		out = append(out, cur.String())
	}
	return out
}
