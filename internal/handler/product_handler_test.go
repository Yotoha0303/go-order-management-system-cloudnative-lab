package handler

import (
	"testing"

	"go-order-management-system/internal/model"
)

func TestParseProductStatus(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantStatus *int8
		wantOK     bool
	}{
		{name: "default off sale", value: "", wantStatus: int8Ptr(model.ProductStatusOffSale), wantOK: true},
		{name: "on sale", value: "1", wantStatus: int8Ptr(model.ProductStatusOnSale), wantOK: true},
		{name: "all", value: "all", wantStatus: nil, wantOK: true},
		{name: "invalid", value: "3", wantStatus: nil, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseProductStatus(tt.value)
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v", tt.wantOK, ok)
			}
			if tt.wantStatus == nil {
				if got != nil {
					t.Fatalf("expected nil status, got %d", *got)
				}
				return
			}
			if got == nil || *got != *tt.wantStatus {
				t.Fatalf("expected status %d, got %v", *tt.wantStatus, got)
			}
		})
	}
}

func TestParseOptionalPagination(t *testing.T) {
	tests := []struct {
		page, pageSize         string
		wantPage, wantPageSize int
		wantOK                 bool
	}{
		{wantOK: true},
		{page: "2", pageSize: "50", wantPage: 2, wantPageSize: 50, wantOK: true},
		{page: "1", wantPage: 1, wantPageSize: 20, wantOK: true},
		{page: "0", pageSize: "20", wantOK: false},
		{page: "1", pageSize: "101", wantOK: false},
	}

	for _, tt := range tests {
		page, pageSize, ok := parseOptionalPagination(tt.page, tt.pageSize)
		if ok != tt.wantOK || page != tt.wantPage || pageSize != tt.wantPageSize {
			t.Fatalf("parse pagination (%q,%q): got (%d,%d,%v), want (%d,%d,%v)",
				tt.page, tt.pageSize, page, pageSize, ok, tt.wantPage, tt.wantPageSize, tt.wantOK)
		}
	}
}

func int8Ptr(value int8) *int8 {
	return &value
}
