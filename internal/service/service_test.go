package service

import (
	"testing"
)

func TestVNCImplementsService(t *testing.T) {
	var _ Service = (*VNC)(nil)
}

func TestVNCName(t *testing.T) {
	v := NewVNC()
	if v.Name() != "VNC" {
		t.Errorf("expected 'VNC', got %q", v.Name())
	}
}

func TestGeneratePassword(t *testing.T) {
	p1 := generatePassword()
	p2 := generatePassword()
	if len(p1) == 0 {
		t.Error("password should not be empty")
	}
	if p1 == p2 {
		t.Error("passwords should be different")
	}
}

func TestLoadStateEmpty(t *testing.T) {
	pw, url := loadState()
	// May or may not have state on dev machine — just verify no panic
	_ = pw
	_ = url
}
