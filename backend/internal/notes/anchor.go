// Package notes checks note anchors and bodies. It does not touch the database.
package notes

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Kind is the part of a snapshot a note is pinned to.
type Kind string

const (
	KindDomain       Kind = "domain"
	KindTable        Kind = "table"
	KindColumn       Kind = "column"
	KindRelationship Kind = "relationship"
	KindLineage      Kind = "lineage"
)

const MaxBodyRunes = 4000

var (
	ErrEmptyBody   = errors.New("note body is empty")
	ErrBodyTooLong = fmt.Errorf("note body is longer than %d characters", MaxBodyRunes)
)

func ParseKind(s string) (Kind, error) {
	switch k := Kind(s); k {
	case KindDomain, KindTable, KindColumn, KindRelationship, KindLineage:
		return k, nil
	}
	return "", fmt.Errorf("unknown anchor kind %q", s)
}

// SplitColumnAnchor splits "<table id>#<column>" on the last '#'.
func SplitColumnAnchor(id string) (tableID, column string, err error) {
	i := strings.LastIndexByte(id, '#')
	if i <= 0 || i == len(id)-1 {
		return "", "", fmt.Errorf("column anchor %q is not <table id>#<column>", id)
	}
	return id[:i], id[i+1:], nil
}

// CheckBody trims the body and refuses it when empty or too long.
func CheckBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ErrEmptyBody
	}
	if utf8.RuneCountInString(body) > MaxBodyRunes {
		return "", ErrBodyTooLong
	}
	return body, nil
}
