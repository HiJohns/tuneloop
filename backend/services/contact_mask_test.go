package services

import "testing"

func TestContactMask_2428(t *testing.T) {
	if got := MaskPhone("13812345678"); got != "138****5678" {
		t.Fatalf("phone mask = %q", got)
	}
	if got := MaskPhone("36547687"); got != "********" {
		t.Fatalf("short phone mask = %q", got)
	}
	if got := MaskEmail("ab@c.cn"); got != "**@c.cn" {
		t.Fatalf("email mask = %q", got)
	}
	if got := MaskEmail("someone@x.com"); got != "so****@x.com" {
		t.Fatalf("long email mask = %q", got)
	}
	if got := MaskUsername("allen"); got != "al****n" {
		t.Fatalf("username mask = %q", got)
	}
	if fields := MatchIdentifier("138", "a@b.c", "u", "138", "", "u"); len(fields) != 2 || fields[0] != "phone" || fields[1] != "username" {
		t.Fatalf("matched fields = %v", fields)
	}
}
