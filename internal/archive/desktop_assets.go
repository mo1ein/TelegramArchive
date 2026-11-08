package archive

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func desktopAssetDirectory(dir string) string {
	base := filepath.Base(dir)
	if strings.HasPrefix(base, "ChatExport_") || base == "saved_messages" || base == "profile_pictures" || base == "stories" {
		return filepath.Dir(dir)
	}
	return dir
}

// Unmodified export assets from Telegram Desktop v7.4.1 (GPL-3.0-or-later).
// https://github.com/telegramdesktop/tdesktop/tree/v7.4.1/Telegram/Resources/export_html
// Copyright (c) 2014-2026 The Telegram Desktop Authors. See licenses/tdesktop-*.
//
//go:embed desktop_assets/css/* desktop_assets/js/* desktop_assets/images/*
var desktopAssets embed.FS

func desktopAssetPaths() []string {
	var paths []string
	fs.WalkDir(desktopAssets, "desktop_assets", func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			paths = append(paths, strings.TrimPrefix(path, "desktop_assets/"))
		}
		return err
	})
	return paths
}

func writeDesktopAssets(dir string) error {
	for _, path := range desktopAssetPaths() {
		data, err := desktopAssets.ReadFile("desktop_assets/" + path)
		if err != nil {
			return err
		}
		destination := filepath.Join(dir, filepath.FromSlash(path))
		if current, err := os.ReadFile(destination); err == nil && bytes.Equal(current, data) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		if err := writeDesktopAsset(destination, data); err != nil {
			return err
		}
	}
	return nil
}

func writeDesktopAsset(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".asset-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
