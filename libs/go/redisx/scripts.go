// Package redisx gom cac tien ich Redis dung chung cho moi service.
package redisx

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

// ScriptSet nap va chay cac Lua script, tu cache SHA.
//
// Moi duong di nong cua he thong (join hang cho, hold ve, xin permit AI) deu la
// mot Lua script atomic. Chay qua EVALSHA thay vi EVAL de khong phai gui lai
// than script hang tram nghin lan moi giay.
type ScriptSet struct {
	rdb     redis.Scripter
	mu      sync.RWMutex
	scripts map[string]*redis.Script
}

func NewScriptSet(rdb redis.Scripter) *ScriptSet {
	return &ScriptSet{rdb: rdb, scripts: make(map[string]*redis.Script)}
}

// LoadFS nap moi file .lua trong fsys, dat ten theo ten file khong duoi mo rong.
func (s *ScriptSet) LoadFS(fsys embed.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("doc thu muc script %q: %w", dir, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		body, err := fsys.ReadFile(path.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("doc script %q: %w", e.Name(), err)
		}
		name := strings.TrimSuffix(e.Name(), ".lua")
		s.scripts[name] = redis.NewScript(string(body))
	}
	return nil
}

// Preload nap san moi script len server luc khoi dong.
//
// Lam viec nay o thoi diem khoi dong chu khong phai o request dau tien: mot
// NOSCRIPT xay ra dung giay mo ban se bien thanh hang nghin lan gui lai than
// script cung luc.
func (s *ScriptSet) Preload(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for name, sc := range s.scripts {
		if err := sc.Load(ctx, s.rdb).Err(); err != nil {
			return fmt.Errorf("nap script %q len redis: %w", name, err)
		}
	}
	return nil
}

// Run chay script theo ten. go-redis tu fallback sang EVAL khi gap NOSCRIPT.
func (s *ScriptSet) Run(ctx context.Context, name string, keys []string, args ...any) *redis.Cmd {
	s.mu.RLock()
	sc, ok := s.scripts[name]
	s.mu.RUnlock()
	if !ok {
		cmd := redis.NewCmd(ctx)
		cmd.SetErr(fmt.Errorf("khong tim thay script %q", name))
		return cmd
	}
	return sc.Run(ctx, s.rdb, keys, args...)
}
