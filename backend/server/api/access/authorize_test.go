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

package access

import (
	"testing"
	"time"

	mockdal "github.com/apache/incubator-devlake/mocks/core/dal"
	"github.com/stretchr/testify/mock"
)

func TestAuthorizeExistingUserPreservesParentDirectoryEmail(t *testing.T) {
	db := &mockdal.Dal{}
	tx := &mockdal.Transaction{}

	db.On("Begin").Return(tx)
	tx.On("Rollback").Return(nil)
	tx.On("Update", mock.Anything).Return(nil)
	tx.On("UpdateColumns", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	tx.On("Commit").Return(nil)

	service := &Service{db: db}

	parentUser := &AccessUser{
		Email:       "alice@company.com",
		DisplayName: "Alice Smith",
		Role:        RoleMember,
		Status:      StatusActive,
	}
	parentUser.ID = 42

	linkedIdentity := &AccessIdentity{
		AccessUserID:  42,
		Issuer:        "https://gitlab.example.com",
		Subject:       "gitlab-sub-123",
		VerifiedEmail: "alice@old-personal.org",
		DisplayName:   "Alice (Old)",
	}

	incomingIdentity := Identity{
		Issuer:      "https://gitlab.example.com",
		Subject:     "gitlab-sub-123",
		Email:       "alice@new-personal.org",
		DisplayName: "Alice Personal",
	}

	principal, err := service.authorizeExistingUser(parentUser, linkedIdentity, incomingIdentity)
	if err != nil {
		t.Fatalf("authorizeExistingUser() error = %v", err)
	}
	if principal.UserID != 42 || principal.Role != RoleMember {
		t.Fatalf("principal = %+v, want UserID 42, Role %s", principal, RoleMember)
	}

	// Critical Invariant: Parent AccessUser directory email and display name MUST NOT be overwritten
	if parentUser.Email != "alice@company.com" {
		t.Fatalf("parentUser.Email = %q, want %q (parent directory email must be preserved)", parentUser.Email, "alice@company.com")
	}
	if parentUser.DisplayName != "Alice Smith" {
		t.Fatalf("parentUser.DisplayName = %q, want %q (parent directory display name must be preserved)", parentUser.DisplayName, "Alice Smith")
	}
	if parentUser.LastLoginAt == nil || time.Since(*parentUser.LastLoginAt) > 5*time.Second {
		t.Fatalf("parentUser.LastLoginAt = %v, want recent timestamp", parentUser.LastLoginAt)
	}

	// Child AccessIdentity claims MUST be updated to reflect the verified identity used for login
	if linkedIdentity.VerifiedEmail != "alice@new-personal.org" {
		t.Fatalf("linkedIdentity.VerifiedEmail = %q, want %q", linkedIdentity.VerifiedEmail, "alice@new-personal.org")
	}
	if linkedIdentity.DisplayName != "Alice Personal" {
		t.Fatalf("linkedIdentity.DisplayName = %q, want %q", linkedIdentity.DisplayName, "Alice Personal")
	}
	if linkedIdentity.LastLoginAt == nil || time.Since(*linkedIdentity.LastLoginAt) > 5*time.Second {
		t.Fatalf("linkedIdentity.LastLoginAt = %v, want recent timestamp", linkedIdentity.LastLoginAt)
	}
}

func TestAuthorizeExistingUserRejectsDisabledOrHiddenUser(t *testing.T) {
	service := &Service{}
	identity := Identity{Issuer: "https://accounts.google.com", Subject: "sub", Email: "alice@company.com"}

	disabledUser := &AccessUser{Status: StatusDisabled}
	disabledUser.ID = 10
	if _, err := service.authorizeExistingUser(disabledUser, &AccessIdentity{}, identity); err == nil {
		t.Fatal("expected disabled user to be rejected")
	}

	now := time.Now()
	hiddenUser := &AccessUser{Status: StatusActive, HiddenAt: &now}
	hiddenUser.ID = 11
	if _, err := service.authorizeExistingUser(hiddenUser, &AccessIdentity{}, identity); err == nil {
		t.Fatal("expected hidden user to be rejected")
	}
}
