package main

import (
	"context"
	"os"
	"testing"
)

func TestResolveServerURL(t *testing.T) {
	ctx := context.Background()

	// 1. Flag value takes highest precedence
	url := resolveServerURL(ctx, "http://192.168.1.100:8080")
	if url != "http://192.168.1.100:8080" {
		t.Fatalf("expected flag URL, got %s", url)
	}

	// 2. Environment variable fallback
	os.Setenv("REDWOLF_SERVER", "http://10.20.30.40:8080")
	defer os.Unsetenv("REDWOLF_SERVER")

	urlEnv := resolveServerURL(ctx, "")
	if urlEnv != "http://10.20.30.40:8080" {
		t.Fatalf("expected env URL, got %s", urlEnv)
	}
}
