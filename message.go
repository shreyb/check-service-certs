package main

import (
	"context"
)

// Message is an interface for types that implement a sendMessage method to send messages
type Message interface {
	sendMessage(ctx context.Context, message string) error
}
