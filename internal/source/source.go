// Package source fetches the encrypted regex bundle from the NetEase SDK
// endpoint, decrypts it, and decodes the msgpack payload into the per-group
// serialized PCRE2 blobs (name -> sregex).
package source

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ulikunitz/xz/lzma"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	initboxURL = "https://optsdk.gameyw.netease.com/initbox_p2_android_g79.html"
	sdkKey     = "c42bf7f39d476db3"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Fetch runs the full pipeline: resolve the bundle URL, download, decrypt,
// decompress, and decode the msgpack into a name -> serialized PCRE2 blob map.
// It also returns the resolved bundle URL so callers can skip reloads when the
// URL has not changed.
func Fetch(ctx context.Context) (groups map[string][]byte, url string, err error) {
	url, err = resolveURL(ctx)
	if err != nil {
		return nil, "", err
	}

	packed, err := download(ctx, url)
	if err != nil {
		return nil, url, err
	}

	groups, err = ParseGroups(packed)
	if err != nil {
		return nil, url, err
	}

	return groups, url, nil
}

// ResolveURL returns the current bundle URL without downloading it. Useful for
// cheaply checking whether a reload is necessary.
func ResolveURL(ctx context.Context) (string, error) {
	return resolveURL(ctx)
}

// Download fetches and decrypts the bundle at url, returning the raw msgpack
// bytes.
func Download(ctx context.Context, url string) ([]byte, error) {
	return download(ctx, url)
}

// ParseGroups decodes the decrypted msgpack payload into a map of group name to
// its serialized PCRE2 blob (the "sregex" field).
func ParseGroups(packed []byte) (map[string][]byte, error) {
	var payload struct {
		Regex map[string]struct {
			SRegex []byte `msgpack:"sregex"`
		} `msgpack:"regex"`
	}

	if err := msgpack.Unmarshal(packed, &payload); err != nil {
		return nil, fmt.Errorf("decode msgpack: %w", err)
	}

	if len(payload.Regex) == 0 {
		return nil, fmt.Errorf("msgpack contains no regex groups")
	}

	groups := make(map[string][]byte, len(payload.Regex))
	for name, item := range payload.Regex {
		if len(item.SRegex) == 0 {
			continue
		}
		groups[name] = item.SRegex
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("msgpack contains no sregex blobs")
	}

	return groups, nil
}

func resolveURL(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		initboxURL,
		nil,
	)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var info struct {
		URL string `json:"url_pdata"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("decode initbox response: %w", err)
	}
	if info.URL == "" {
		return "", fmt.Errorf("initbox response has no url_pdata")
	}

	return info.URL, nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return decrypt(body)
}

// decrypt reverses the SDK encoding: AES-256-CBC (key = sdkKey repeated, IV =
// the first 16 bytes), then LZMA decompression, yielding the msgpack payload.
func decrypt(raw []byte) ([]byte, error) {
	if len(raw) < aes.BlockSize {
		return nil, fmt.Errorf("encrypted payload too short: %d bytes", len(raw))
	}

	key := []byte(sdkKey + sdkKey)
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid key length: %d", len(key))
	}

	iv := raw[:aes.BlockSize]
	ciphertext := raw[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length %d not a multiple of block size", len(ciphertext))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)

	plain, err = pkcs7Unpad(plain)
	if err != nil {
		return nil, err
	}

	return lzmaDecode(plain)
}

func pkcs7Unpad(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("empty plaintext")
	}

	pad := int(src[len(src)-1])
	if pad <= 0 || pad > aes.BlockSize || pad > len(src) {
		return nil, fmt.Errorf("invalid padding")
	}

	return src[:len(src)-pad], nil
}

func lzmaDecode(data []byte) ([]byte, error) {
	r, err := lzma.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("lzma reader: %w", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("lzma decode: %w", err)
	}

	return out, nil
}
