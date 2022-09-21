package main

import (
	"strconv"
	"testing"

	"github.com/sorenmat/k8s-rds/crd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExcluded(t *testing.T) {
	tests := []struct {
		name     string
		db       *crd.Database
		exclNS   []string
		inclNS   []string
		excluded bool
	}{
		{
			name:     "no excluded or included namespaces",
			db:       &crd.Database{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}},
			excluded: false,
		},
		{
			name:     "namespace not in excluded namespaces",
			db:       &crd.Database{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}},
			exclNS:   []string{"test"},
			excluded: false,
		},
		{
			name:     "namespace not in included namespaces",
			db:       &crd.Database{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}},
			inclNS:   []string{"test"},
			excluded: true,
		},
		{
			name:     "namespace in excluded namespaces",
			db:       &crd.Database{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}},
			exclNS:   []string{"default"},
			excluded: true,
		},
		{
			name:     "namespace in included namespaces",
			db:       &crd.Database{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}},
			inclNS:   []string{"default"},
			excluded: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expected := test.excluded
			if actual := excluded(test.db.Namespace, test.db.Name, test.exclNS, test.inclNS); actual != expected {
				t.Errorf("expected %v, actual %v", expected, actual)
			}
		})
	}
}

func TestStringInSlice(t *testing.T) {
	tests := []struct {
		str      string
		slice    []string
		expected bool
	}{
		{"", nil, false},
		{"test", nil, false},
		{"", []string{}, false},
		{"test", []string{}, false},
		{"test", []string{"hello"}, false},
		{"test", []string{"test"}, true},
		{"", []string{"test"}, false},
		{"", []string{"test", ""}, true},
		{"test", []string{"test", "test"}, true},
		{"test", []string{"hello", "test"}, true},
		{"hello", []string{"hello", "test"}, true},
		{"world", []string{"hello", "test"}, false},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			expected := test.expected
			if actual := stringInSlice(test.str, test.slice); actual != expected {
				t.Errorf("expected %v, actual %v", expected, actual)
			}
		})
	}
}
