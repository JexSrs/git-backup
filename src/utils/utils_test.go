package utils

import (
	"reflect"
	"testing"
)

func TestPointer_DoesNotAliasOriginalVariable(t *testing.T) {
	v := 42
	p := Pointer(v)
	v = 7

	if *p != 42 {
		t.Fatalf("expected pointer to keep the value it was created with (42), got %d", *p)
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want []int
	}{
		{"odd length", []int{1, 2, 3}, []int{3, 2, 1}},
		{"even length", []int{1, 2, 3, 4}, []int{4, 3, 2, 1}},
		{"single element", []int{9}, []int{9}},
		{"empty", []int{}, []int{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Reverse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Reverse(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestReverse_NilStaysEmpty(t *testing.T) {
	if got := Reverse[int](nil); len(got) != 0 {
		t.Fatalf("expected empty result for nil input, got %v", got)
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		value string
		want  bool
	}{
		{"exact match", []string{"foo", "bar"}, "foo", true},
		{"value differs in case", []string{"foo", "bar"}, "FOO", true},
		{"slice entry differs in case", []string{"OpenSearch"}, "opensearch", true},
		{"no match", []string{"foo", "bar"}, "baz", false},
		{"substring is not a match", []string{"foobar"}, "foo", false},
		{"empty slice", []string{}, "foo", false},
		{"nil slice", nil, "foo", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContainsIgnoreCase(tc.slice, tc.value); got != tc.want {
				t.Fatalf("ContainsIgnoreCase(%v, %q) = %v, want %v", tc.slice, tc.value, got, tc.want)
			}
		})
	}
}

func TestExtractNameFromGitURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://git.proxmox.com/git/lxc.git", "lxc"},
		{"https://github.com/user/repo", "repo"},
		{"https://github.com/user/repo/", "repo"},
		{"https://github.com/user/repo.git/", "repo"},
		{"git@github.com:user/repo.git", "repo"},
		{"plain", "plain"},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			if got := ExtractNameFromGitURL(tc.url); got != tc.want {
				t.Fatalf("ExtractNameFromGitURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestCheckPrefix(t *testing.T) {
	tests := []struct {
		id     string
		prefix string
		want   bool
	}{
		{"gitlab", "gitlab", true},
		{"gitlab-custom-1", "gitlab", true},
		{"git-proxmox", "git", true},
		{"gitlab", "git", false},
		{"github", "git", false},
		{"gitlabx", "gitlab", false},
		{"gitlab_custom", "gitlab", false},
		{"", "gitlab", false},
	}

	for _, tc := range tests {
		t.Run(tc.id+"/"+tc.prefix, func(t *testing.T) {
			if got := CheckPrefix(tc.id, tc.prefix); got != tc.want {
				t.Fatalf("CheckPrefix(%q, %q) = %v, want %v", tc.id, tc.prefix, got, tc.want)
			}
		})
	}
}
