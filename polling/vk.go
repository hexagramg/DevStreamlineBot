package polling

import (
	"context"
	"fmt"
	"log"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	"gorm.io/gorm"

	"devstreamlinebot/models"
)

type VKEvent struct {
	Msg  *botgolang.Message
	From botgolang.Contact
}

func StartVKPolling(db *gorm.DB, baseURL, token string) (*botgolang.Bot, <-chan VKEvent) {
	vkBot, err := botgolang.NewBot(token, botgolang.BotApiURL(baseURL))
	if err != nil {
		log.Fatalf("failed to create VK bot: %v", err)
	}
	events := make(chan VKEvent)
	go func() {
		ctx := context.Background()
		updates := vkBot.GetUpdatesChannel(ctx)
		for update := range updates {
			if update.Type == botgolang.NEW_MESSAGE {
				msg := update.Payload.Message()
				chatIDStr := fmt.Sprint(msg.Chat.ID)
				var chat models.Chat
				chatData := models.Chat{ChatID: chatIDStr, Type: msg.Chat.Type, Title: msg.Chat.Title}
				if err := db.Where(models.Chat{ChatID: chatIDStr}).Assign(chatData).FirstOrCreate(&chat).Error; err != nil {
					log.Printf("Error upserting VK chat %s: %v", chatIDStr, err)
				}

				userIDStr := fmt.Sprint(update.Payload.From.ID)
				var user models.VKUser
				vkUserData := models.VKUser{UserID: userIDStr, FirstName: update.Payload.From.FirstName, LastName: update.Payload.From.LastName}
				if err := db.Where(models.VKUser{UserID: userIDStr}).Assign(vkUserData).FirstOrCreate(&user).Error; err != nil {
					log.Printf("Error upserting VK user %s: %v", userIDStr, err)
				}

				vkMsg := models.VKMessage{MessageID: msg.ID, ChatID: chat.ID, UserID: user.ID, Text: msg.Text, Timestamp: time.Unix(int64(msg.Timestamp), 0)}
				if err := db.Create(&vkMsg).Error; err != nil {
					log.Printf("Error storing VK message for chat %s, user %s: %v", chat.ChatID, user.UserID, err)
				}
				events <- VKEvent{Msg: msg, From: update.Payload.From}
			}
		}
	}()
	return vkBot, events
}
