package dnssd

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

type genericop interface {
	Start() error
	Active() bool
	Stop()
}

func StartStopHelper(t *testing.T, op genericop) {
	if err := op.Start(); err != nil {
		t.Fatalf("Couldn't start op: %v", err)
	}
	if !op.Active() {
		t.Fatal("Op didn't become active")
	}
	op.Stop()
	if op.Active() {
		t.Fatal("Op didn't become inactive")
	}
}

func TestRegisterStartStop(t *testing.T) {
	f := func(op *RegisterOp, e error, add bool, name, serviceType, domain string) {
	}
	StartStopHelper(t, NewRegisterOp("go", "_go-dnssd._tcp", 9, f))
}

func TestRegTxt(t *testing.T) {
	op := &RegisterOp{}
	key, value := "a", "a"
	if err := op.SetTXTPair(key, value); err != nil {
		t.Fatalf(`Unexpected error setting key "%s", value "%s": %v`, key, value, err)
	}
	if l := 2 + len(key) + len(value); op.txt.l != l {
		t.Fatalf(`Expected length %d after setting key "%s", value "%s", got: %d`, l, key, value, op.txt.l)
	}
	if err := op.DeleteTXTPair(key); err != nil {
		t.Fatalf(`Unexpected error deleting key "%s": %v`, key, err)
	}
	if op.txt.l != 0 {
		t.Fatalf(`Expected length 0 after deleting key "%s, got: %v`, key, op.txt.l)
	}
	key = strings.Repeat("a", 128)
	value = key
	if err := op.SetTXTPair(key, value); err != ErrTXTStringLen {
		t.Fatalf("Expected ErrTXTStringLen, got: %v", err)
	}
	key = strings.Repeat("a", 126)
	value = key
	ssize := 2 + len(key) + len(value)
	for i := 0; i < (65535 / ssize); i++ {
		key := fmt.Sprintf("%0126d", i)
		if err := op.SetTXTPair(key, value); err != nil {
			t.Fatalf("Unexpected error setting up for ErrTXTLen: %v", err)
		}
	}
	if err := op.SetTXTPair(key, value); err != ErrTXTLen {
		t.Fatalf("Expected ErrTXTLen, got: %v", err)
	}
	for i := 0; i < (65535 / ssize); i++ {
		key := fmt.Sprintf("%0126d", i)
		if err := op.DeleteTXTPair(key); err != nil {
			t.Fatalf("Unexpected error tearing down from ErrTXTLen test: %v", err)
		}
	}
	if op.txt.l != 0 {
		t.Fatalf("Expected length 0 after tearing down from ErrTXTLen test, got: %v", op.txt.l)
	}
}

func TestBrowseStartStop(t *testing.T) {
	f := func(op *BrowseOp, e error, add bool, interfaceIndex int, name string, serviceType string, domain string) {
	}
	StartStopHelper(t, NewBrowseOp("_go-dnssd._tcp", f))
}

func TestResolveStartStop(t *testing.T) {
	f := func(op *ResolveOp, e error, host string, port int, txt map[string]string) {
	}
	StartStopHelper(t, NewResolveOp(0, "go", "_go-dnssd._tcp", "local", f))
}

func TestDecodeTxtBadLength(t *testing.T) {
	b := []byte{255, 'b', '=', 'b'}
	m := decodeTxt(b)
	if v, p := m["b"]; p != false {
		t.Fatalf(`Expected pair "b" to be missing, instead it's present with value %v`, v)
	}
}

func TestDecodeTxtKeyNoValue(t *testing.T) {
	b := []byte{1, 'a', 2, 'b', '=', 1, '=', 2, '=', 'a'}
	m := decodeTxt(b)
	keys := []string{"a", "b", "=", "=a"}
	for _, k := range keys {
		if v, p := m[k]; v != "" {
			t.Fatalf(`Expected "%s" to return empty string, got %v instead (present: %v)`, k, v, p)
		}
	}
}

func TestDecodeTxtKeyValue(t *testing.T) {
	b := []byte{3, 'a', '=', 'a', 3, 'b', '=', 'b', 5, 'a', 'b', '=', 'a', 'b'}
	m := decodeTxt(b)
	for _, kv := range []string{"a", "b", "ab"} {
		if v, p := m[kv]; v != kv {
			t.Fatalf(`Expected "%s" to return "%s", got %v instead (present: %v)`, kv, kv, v, p)
		}
	}
}

func TestQueryStartStop(t *testing.T) {
	f := func(op *QueryOp, err error, add, more bool, interfaceIndex int, fullname string, rrtype, rrclass uint16, rdata []byte, ttl uint32) {
	}
	StartStopHelper(t, NewQueryOp(0, "golang.org.", 1, 1, f))
}

func TestRegisterPort(t *testing.T) {
	sport := 0xCAFE
	sname := "go-dnssd-test"
	stype := "_" + sname + "._udp"
	sdom := "local"
	errch := make(chan string)
	senderr := func(f string, a ...interface{}) {
		select {
		case errch <- fmt.Sprintf(f, a...):
		default:
		}
	}
	resop := NewResolveOp(InterfaceIndexLocalOnly, sname, stype, sdom, func(op *ResolveOp, err error, host string, port int, txt map[string]string) {
		switch {
		case err != nil:
			senderr("resolve callback - error: %s", err)
		case port != sport:
			senderr("resolve callback - bad port. expected: %d got: %d", sport, port)
		default:
			t.Logf("resolve callback - called with correct port")
			close(errch)
		}
	})
	regop := NewRegisterOp(sname, stype, sport, func(op *RegisterOp, err error, add bool, name, serviceType, domain string) {
		action := "rmv"
		switch {
		case err != nil:
			senderr("register callback - error: %s", err)
		case add:
			action = "add"
			fallthrough
		default:
			t.Logf("register callback - %s", action)
			switch err := resop.Start(); err {
			case nil:
				t.Logf("register callback - resolve op started")
			case ErrStarted:
			default:
				senderr("register callback - resolve op start failed: %s", err)
			}
		}
	})
	if err := regop.SetNoAutoRename(true); err != nil {
		panic(err)
	}
	if err := regop.SetInterfaceIndex(InterfaceIndexLocalOnly); err != nil {
		panic(err)
	}
	if err := regop.SetDomain("local"); err != nil {
		panic(err)
	}
	if err := regop.Start(); err != nil {
		t.Fatalf("register op start failed: %s", err)
	} else {
		t.Logf("register op started")
	}
	defer regop.Stop()
	defer resop.Stop()
	go func() {
		time.Sleep(time.Second)
		senderr("test took longer than a second")
	}()
	if errmsg, ok := <-errch; ok {
		t.Fatal(errmsg)
	}
}
