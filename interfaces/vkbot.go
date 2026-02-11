package interfaces

import botgolang "github.com/mail-ru-im/bot-golang"

// VKBotMessage represents a message that can be sent.
type VKBotMessage interface {
	Send() error
}

// VKBot abstracts VK Teams bot operations for testing.
type VKBot interface {
	NewTextMessage(chatID string, text string) VKBotMessage
	NewHTMLMessage(chatID string, text string) VKBotMessage
}

// RealVKBot wraps *botgolang.Bot to implement VKBot interface.
type RealVKBot struct {
	Bot *botgolang.Bot
}

// NewTextMessage creates a new text message.
func (r *RealVKBot) NewTextMessage(chatID string, text string) VKBotMessage {
	return r.Bot.NewTextMessage(chatID, text)
}

func (r *RealVKBot) NewHTMLMessage(chatID string, text string) VKBotMessage {
	msg := r.Bot.NewTextMessage(chatID, text)
	msg.ParseMode = botgolang.ParseModeHTML
	return msg
}
