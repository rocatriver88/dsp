package alert

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDingTalkSend(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()

	d := NewDingTalk(srv.URL)
	err := d.Send("test title", "test content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgType, _ := received["msgtype"].(string)
	if msgType != "markdown" {
		t.Errorf("expected markdown, got %s", msgType)
	}
	md, _ := received["markdown"].(map[string]any)
	title, _ := md["title"].(string)
	if title != "test title" {
		t.Errorf("expected 'test title', got %s", title)
	}
}

func TestFeishuSend(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	f := NewFeishu(srv.URL)
	err := f.Send("test title", "test content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgType, _ := received["msg_type"].(string)
	if msgType != "interactive" {
		t.Errorf("expected interactive, got %s", msgType)
	}

	card, _ := received["card"].(map[string]any)
	header, _ := card["header"].(map[string]any)
	titleObj, _ := header["title"].(map[string]any)
	titleContent, _ := titleObj["content"].(string)
	if titleContent != "test title" {
		t.Errorf("expected card header title 'test title', got %q", titleContent)
	}

	elements, _ := card["elements"].([]any)
	if len(elements) == 0 {
		t.Fatal("expected at least one element in card.elements")
	}
	elem, _ := elements[0].(map[string]any)
	elemContent, _ := elem["content"].(string)
	if elemContent != "test content" {
		t.Errorf("expected card element content 'test content', got %q", elemContent)
	}
}

func TestNoop(t *testing.T) {
	n := Noop{}
	if err := n.Send("a", "b"); err != nil {
		t.Errorf("noop should not error: %v", err)
	}
}
