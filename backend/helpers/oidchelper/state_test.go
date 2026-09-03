/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package oidchelper

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEncodeDecodeStateRoundTrip(t *testing.T) {
	secret := []byte("a-test-secret-with-at-least-32-bytes!")
	in := &StatePayload{
		Nonce:               "abc123",
		ReturnURL:           "/projects",
		PKCEVerifier:        "verifier",
		IdentityLinkStateID: "link-state-id",
		IssuedAt:            time.Now(),
	}
	encoded, err := EncodeState(secret, in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := DecodeState(secret, encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Nonce != in.Nonce || out.ReturnURL != in.ReturnURL || out.PKCEVerifier != in.PKCEVerifier || out.IdentityLinkStateID != in.IdentityLinkStateID {
		t.Fatalf("round-trip mismatch: %+v vs %+v", in, out)
	}
}

func TestDecodeStateRejectsTamperedCiphertext(t *testing.T) {
	secret := []byte("a-test-secret-with-at-least-32-bytes!")
	encoded, err := EncodeState(secret, &StatePayload{Nonce: "n", IssuedAt: time.Now()})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Flip a char in the middle of the ciphertext (past the AES-GCM nonce
	// prefix); flipping the very last char of base64 can be a no-op when
	// the trailing bits are unused, defeating the test.
	mid := len(encoded) / 2
	tampered := encoded[:mid] + flipChar(encoded[mid]) + encoded[mid+1:]
	if _, err := DecodeState(secret, tampered); err == nil {
		t.Fatal("expected decode to fail on tampered ciphertext")
	}
}

func TestDecodeStateRejectsWrongSecret(t *testing.T) {
	encoded, err := EncodeState([]byte("first-secret-with-at-least-32-bytes!"), &StatePayload{Nonce: "n", IssuedAt: time.Now()})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, err := DecodeState([]byte("other-secret-with-at-least-32-bytes!"), encoded); err == nil {
		t.Fatal("expected decode to fail with a different secret")
	}
}

func TestDecodeStateRejectsExpiredPayload(t *testing.T) {
	secret := []byte("a-test-secret-with-at-least-32-bytes!")
	encoded, err := EncodeState(secret, &StatePayload{
		Nonce:    "n",
		IssuedAt: time.Now().Add(-30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	_, err = DecodeState(secret, encoded)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got: %v", err)
	}
}

func TestNewNonceUniqueness(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 50; i++ {
		n, err := NewNonce()
		if err != nil {
			t.Fatalf("NewNonce: %v", err)
		}
		if len(n) < 20 {
			t.Fatalf("nonce too short: %q", n)
		}
		if _, dup := seen[n]; dup {
			t.Fatalf("duplicate nonce: %q", n)
		}
		seen[n] = struct{}{}
	}
}

func flipChar(b byte) string {
	if b == 'A' {
		return "B"
	}
	return "A"
}

func TestStatePayloadHistoricalDecodeWithoutLinkStateID(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	// Historical JSON payload minted before IdentityLinkStateID was added.
	historicalJSON := []byte(`{"v":"google","n":"test-nonce","r":"/dashboards","p":"test-pkce-verifier","t":"` + now.Format(time.RFC3339Nano) + `"}`)

	var payload StatePayload
	if err := json.Unmarshal(historicalJSON, &payload); err != nil {
		t.Fatalf("unmarshal historical payload: %v", err)
	}
	if payload.Provider != "google" || payload.Nonce != "test-nonce" || payload.ReturnURL != "/dashboards" || payload.PKCEVerifier != "test-pkce-verifier" {
		t.Fatalf("historical payload fields mismatch: %+v", payload)
	}
	if payload.IdentityLinkStateID != "" {
		t.Fatalf("expected empty IdentityLinkStateID on historical payload, got %q", payload.IdentityLinkStateID)
	}

	// Verify that marshaling a payload with empty IdentityLinkStateID omits the "l" key.
	marshaled, err := json.Marshal(&payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(marshaled), `"l"`) {
		t.Fatalf("expected empty IdentityLinkStateID to be omitted, got %s", string(marshaled))
	}

	// Verify full encode/decode round trip with empty IdentityLinkStateID
	secret := []byte("a-test-secret-with-at-least-32-bytes!")
	encoded, err := EncodeState(secret, &payload)
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	decoded, err := DecodeState(secret, encoded)
	if err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if decoded.IdentityLinkStateID != "" {
		t.Fatalf("expected empty IdentityLinkStateID after decode, got %q", decoded.IdentityLinkStateID)
	}
}
