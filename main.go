package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/rs/zerolog/log"
)

type ChatExport struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	ID       int64     `json:"id"`
	Messages []Message `json:"messages"`
}

type Message struct {
	ID           int64          `json:"id"`
	Type         string         `json:"type"` // "message", "service"
	Date         time.Time      `json:"-"`
	From         string         `json:"from,omitempty"`
	FromID       string         `json:"from_id,omitempty"`
	Text         string         `json:"-"`             // final parsed text
	TextEntities []TextFragment `json:"text_entities"` // final parsed text
	// ReplyToMessageID int64     `json:"reply_to_message_id,omitempty"`
	// Edited           string    `json:"edited,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Photo     string `json:"photo,omitempty"`
	// File            *File      `json:"file,omitempty"`
	// Audio           *Audio     `json:"audio,omitempty"`
	// Video           *Video     `json:"video,omitempty"`
	// Sticker         *Sticker   `json:"sticker,omitempty"`
	// Contact         *Contact   `json:"contact,omitempty"`
	// Location        *Location  `json:"location,omitempty"`
	// Poll            *Poll      `json:"poll,omitempty"`
	ForwardedFrom string `json:"forwarded_from,omitempty"`
	// ForwardedFromID string     `json:"forwarded_from_id,omitempty"`
	Reactions []Reaction `json:"reactions,omitempty"`
}

// parts of composite text
type TextFragment struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (m *Message) UnmarshalJSON(data []byte) error {
	// alias to prevent recursion
	type alias Message
	aux := &struct {
		Text    json.RawMessage `json:"text"`
		RawDate string          `json:"date"`
		RawUnix string          `json:"date_unixtime"`

		*alias
	}{
		alias: (*alias)(m),
	}

	// decode everything but text
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	t, err := time.Parse("2006-01-02T15:04:05", aux.RawDate)
	if err != nil {
		return err
	}
	m.Date = t

	// 1) TEXT = "string"
	var s string
	if err := json.Unmarshal(aux.Text, &s); err == nil {
		m.Text = s
		return nil
	}

	// 2) TEXT = array (mixed types)
	var arr []interface{}
	if err := json.Unmarshal(aux.Text, &arr); err == nil {
		out := ""

		for _, item := range arr {

			switch v := item.(type) {

			// item = "string"
			case string:
				out += v

			// item = { "type": "...", "text": "..." }
			case map[string]interface{}:
				if t, ok := v["text"].(string); ok {
					out += t
				}

			// ignore unexpected types
			default:
				// do nothing
			}
		}

		m.Text = out
		return nil
	}

	// unknown but non-critical — treat as empty text
	m.Text = ""
	return nil
}

type Photo struct {
	File      string `json:"file"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

type File struct {
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	MimeType string `json:"mime_type"`
}

type Audio struct {
	FileName  string `json:"file_name"`
	Duration  int    `json:"duration"`
	Performer string `json:"performer,omitempty"`
	Title     string `json:"title,omitempty"`
}

type Video struct {
	FileName string `json:"file_name"`
	Duration int    `json:"duration"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type Sticker struct {
	Emoji string `json:"emoji"`
	File  string `json:"file"`
}

type Contact struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Phone     string `json:"phone_number"`
}

type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Poll struct {
	Question string       `json:"question"`
	Answers  []PollAnswer `json:"answers"`
}

type PollAnswer struct {
	Text   string `json:"text"`
	Voters int    `json:"voters"`
}

type Reaction struct {
	Emoji  string `json:"emoji"` // сам эмодзи
	Count  int    `json:"count"` // сколько всего таких реакций на сообщении
	Type   string `json:"type"`  // "emoji" и т.п.
	Recent []struct {
		From   string `json:"from"`    // имя пользователя, который поставил реакцию
		FromID string `json:"from_id"` // id пользователя
		Date   string `json:"date"`    // дата реакции
	} `json:"recent"` // кто ставил реакцию недавно
}

func filterMessages(msg []Message, filters ...func(Message) bool) []Message {
	res := []Message{}
	for _, m := range msg {
		pass := true
		for _, f := range filters {
			if !f(m) {
				pass = false
				break
			}
		}
		if pass {
			res = append(res, m)
		}
	}
	return res
}

func readFile(fileName string) (*ChatExport, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("cannot read file: %w", err)
	}

	var export ChatExport

	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	return &export, nil
}

func generateHTML(inFile, outFile string, data PageData) error {
	t, err := template.ParseFiles(inFile)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return fmt.Errorf("exec template: %w", err)
	}

	if err := os.WriteFile(outFile, out.Bytes(), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

type Nomination struct {
	Title    string // заголовок номинации
	Avatar   string // URL аватарки (может быть data URL)
	Subtitle string // число или дата
	Caption  string // подпись/комментарий
}

type PageData struct {
	Title       string
	Nominations []Nomination
}

const defaultAvatar = "images/1.jpg"

func userAvatar(id string) string {
	return fmt.Sprintf("images/%s.jpg", id)
}

func labelID(m Message) string  { return m.FromID }
func labelDay(m Message) string { return m.Date.Format("Monday, 2 January") }

func filterTrue(m Message) bool        { return true }
func filterVideo(m Message) bool       { return m.MediaType == "video_message" }
func filterTextMsg(m Message) bool     { return m.MediaType == "" && m.Text != "" }
func filterTikTok(m Message) bool      { return strings.Contains(m.Text, "tiktok.com") }
func filterTypeMessage(m Message) bool { return m.Type == "message" }
func filterForwarded(m Message) bool   { return m.ForwardedFrom != "" }
func filterYear(year int) func(m Message) bool {
	return func(m Message) bool {
		return m.Date.Year() == year
	}
}

func most(userCounts map[string]int, findMax bool) (string, int) {
	if len(userCounts) == 0 {
		return "", 0
	}

	var targetUser string
	var targetValue int
	first := true

	for user, count := range userCounts {
		if first {
			targetUser = user
			targetValue = count
			first = false
			continue
		}
		if findMax && count > targetValue {
			targetUser = user
			targetValue = count
		} else if !findMax && count < targetValue {
			targetUser = user
			targetValue = count
		}
	}

	return targetUser, targetValue
}

func count(msg []Message, filter func(Message) bool, label func(Message) string) map[string]int {
	cnt := map[string]int{}
	for _, m := range msg {
		if filter(m) {
			cnt[label(m)]++
		}
	}
	return cnt
}

func messagesTotal(msg []Message) Nomination {
	return Nomination{
		Title:    "Всего сообщений",
		Avatar:   defaultAvatar,
		Subtitle: fmt.Sprintf("%d сообщений", len(msg)),
		Caption:  "было написано в срамной жопе за год",
	}
}

func mostTotalUser(msg []Message) Nomination {
	userCount := count(msg, filterTrue, labelID)
	user, cnt := most(userCount, true)

	return Nomination{
		Title:    "Самый активный",
		Subtitle: fmt.Sprintf("%d", cnt),
		Caption:  "сообщений за год",
		Avatar:   userAvatar(user),
	}
}

func firstMessage(msg []Message) Nomination {
	textMsg := filterMessages(msg, filterTextMsg)

	first := textMsg[0]

	return Nomination{
		Title:    "Первое сообщение в этом году",
		Subtitle: first.Date.Format(time.DateTime),
		Caption:  first.Text,
		Avatar:   userAvatar(first.FromID),
	}
}

func minTotalUser(msg []Message) Nomination {
	userCount := count(msg, filterTrue, labelID)
	user, cnt := most(userCount, false)

	return Nomination{
		Title:    "Самый молчаливый :(",
		Subtitle: fmt.Sprintf("%d", cnt),
		Caption:  "всего сообщений за год",
		Avatar:   userAvatar(user),
	}
}

func maxVideo(msg []Message) Nomination {
	userCount := count(msg, filterVideo, labelID)
	user, cnt := most(userCount, true)
	return Nomination{
		Title:    "Король подкастов",
		Subtitle: fmt.Sprintf("%d", cnt),
		Caption:  "кружков записано за год",
		Avatar:   userAvatar(user),
	}
}

func maxTikTok(msg []Message) Nomination {
	userCount := count(msg, filterTikTok, labelID)
	user, cnt := most(userCount, true)
	return Nomination{
		Title:    "Айпад-кид года",
		Subtitle: fmt.Sprintf("%d", cnt),
		Caption:  "скинул тиктоков за год",
		Avatar:   userAvatar(user),
	}
}

func maxForward(msg []Message) Nomination {
	userCount := count(msg, filterForwarded, labelID)
	user, cnt := most(userCount, true)
	return Nomination{
		Title:    "Они любили сплетничать",
		Subtitle: fmt.Sprintf("%d", cnt),
		Caption:  "переслал сообщений за год",
		Avatar:   userAvatar(user),
	}
}

func maxDay(msg []Message) Nomination {
	dayCount := count(msg, filterTrue, labelDay)
	day, cnt := most(dayCount, true)
	return Nomination{
		Title:    "Базарили больше всего",
		Subtitle: day,
		Caption:  fmt.Sprintf("%d сообщений за день", cnt),
		Avatar:   defaultAvatar,
	}
}

func championByDays(msg []Message) Nomination {
	// мапа: пользователь → множество дней
	userDays := map[string]map[string]struct{}{}

	for _, m := range msg {
		if m.FromID == "" {
			continue
		}
		day := m.Date.Format("2006-01-02") // уникальный день
		if _, ok := userDays[m.FromID]; !ok {
			userDays[m.FromID] = map[string]struct{}{}
		}
		userDays[m.FromID][day] = struct{}{}
	}

	// мапа: пользователь → количество дней
	countDays := map[string]int{}
	for user, days := range userDays {
		countDays[user] = len(days)
	}

	user, cnt := most(countDays, true) // ищем максимальное количество дней

	return Nomination{
		Title:    "Чемпион по дням",
		Subtitle: fmt.Sprintf("%d дней активности", cnt),
		Caption:  "писал почти каждый день в году",
		Avatar:   userAvatar(user),
	}
}

func longestWriter(msg []Message) Nomination {
	userTotalLength := map[string]int{}
	userMsgCount := map[string]int{}

	for _, m := range msg {
		if m.FromID == "" || m.Text == "" {
			continue
		}
		userTotalLength[m.FromID] += len(m.Text)
		userMsgCount[m.FromID]++
	}

	avgLength := map[string]int{}
	for user, total := range userTotalLength {
		avgLength[user] = total / userMsgCount[user]
	}

	user, avg := most(avgLength, true) // ищем максимальную среднюю длину

	return Nomination{
		Title:    "Самый длинный рассказчик",
		Subtitle: fmt.Sprintf("%d символов в среднем", avg),
		Caption:  "пишет самые длинные сообщения",
		Avatar:   userAvatar(user),
	}
}

func maxStickers(msg []Message) Nomination {
	userCount := map[string]int{}

	for _, m := range msg {
		if m.FromID == "" {
			continue
		}
		if m.MediaType == "sticker" { // если используем MediaType
			userCount[m.FromID]++
		}
		// если используем поле Sticker:
		// if m.Sticker != nil {
		//     userCount[m.FromID]++
		// }
	}

	user, cnt := most(userCount, true)

	return Nomination{
		Title:    "Коллекционер стикеров",
		Subtitle: fmt.Sprintf("%d стикеров", cnt),
		Caption:  "отправил стикеров за год",
		Avatar:   userAvatar(user),
	}
}

// подсчёт количества эмодзи в строке
func isEmoji(r rune) bool {
	// диапазоны для эмодзи (часто используемые)
	return (r >= 0x1F600 && r <= 0x1F64F) || // эмоции
		(r >= 0x1F300 && r <= 0x1F5FF) || // символы и пиктограммы
		(r >= 0x1F680 && r <= 0x1F6FF) || // транспорт и карты
		(r >= 0x2600 && r <= 0x26FF) || // символы Misc
		(r >= 0x2700 && r <= 0x27BF) // символы Misc Dingbats
}

func countEmoji(s string) int {
	count := 0
	for _, r := range s {
		if isEmoji(r) {
			count++
		}
	}
	return count
}

func emojiMaster(msg []Message) Nomination {
	userCount := map[string]int{}

	for _, m := range msg {
		if m.FromID == "" || m.Text == "" {
			continue
		}
		userCount[m.FromID] += countEmoji(m.Text)
	}

	user, cnt := most(userCount, true)

	return Nomination{
		Title:    "Миллинеал года",
		Subtitle: fmt.Sprintf("%d эмодзи", cnt),
		Caption:  "использовал эмодзи в этом году",
		Avatar:   userAvatar(user),
	}
}

func mostUsedEmoji(msg []Message) Nomination {
	emojiCount := map[string]int{}

	for _, m := range msg {
		if m.Text == "" {
			continue
		}
		for _, r := range m.Text {
			if isEmoji(r) {
				emojiCount[string(r)]++
			}
		}
	}

	emoji, cnt := most(emojiCount, true) // используем уже существующую функцию most

	return Nomination{
		Title:    "Ты умрешь и т.д.",
		Subtitle: fmt.Sprintf("эмоджи %s", emoji),
		Caption:  fmt.Sprintf("использовался %d раз", cnt),
		Avatar:   defaultAvatar, // можно оставить общую аватарку
	}
}

func mostReactions(msg []Message) Nomination {
	userCount := map[string]int{}

	for _, m := range msg {
		if m.FromID == "" || len(m.Reactions) == 0 {
			continue
		}
		total := 0
		for _, r := range m.Reactions {
			total += r.Count
		}
		userCount[m.FromID] += total
	}

	user, cnt := most(userCount, true)

	return Nomination{
		Title:    "Приз зрительских симпатий",
		Subtitle: fmt.Sprintf("%d реакций", cnt),
		Caption:  "получил больше всего реакций за год",
		Avatar:   userAvatar(user),
	}
}

func mostGivenReactions(msg []Message) Nomination {
	userCount := map[string]int{}

	for _, m := range msg {
		if len(m.Reactions) == 0 {
			continue
		}
		for _, r := range m.Reactions {
			for _, recent := range r.Recent {
				userCount[recent.FromID]++
			}
		}
	}

	user, cnt := most(userCount, true)

	return Nomination{
		Title:    "Тихий согл...",
		Subtitle: fmt.Sprintf("%d реакций", cnt),
		Caption:  "поставил больше всех реакций за год",
		Avatar:   userAvatar(user),
	}
}

func maxPhotos(msg []Message) Nomination {
	userCount := map[string]int{}

	for _, m := range msg {
		if m.Photo != "" {
			userCount[m.FromID]++
		}
	}

	user, cnt := most(userCount, true)

	return Nomination{
		Title:    "Фотограф года",
		Subtitle: fmt.Sprintf("%d фото", cnt),
		Caption:  "скинул больше всех фото за год",
		Avatar:   userAvatar(user),
	}
}

func mostMentioned(msg []Message) Nomination {
	mentionCount := map[string]int{}

	for _, m := range msg {
		// Предположим, что у тебя есть поле m.TextEntities []TextEntity
		for _, ent := range m.TextEntities {
			if ent.Type == "mention" && ent.Text != "" {
				// убираем символ @
				user := strings.TrimPrefix(ent.Text, "@")
				mentionCount[user]++
			}
		}
	}

	user, cnt := most(mentionCount, true)

	return Nomination{
		Title:    "Любимец чата",
		Subtitle: fmt.Sprintf("%d упоминаний @%s", cnt, user),
		Caption:  "его чаще всех тегали через @",
		// хардкод
		Avatar: userAvatar("user1097835763"),
	}
}

func formPage(msg []Message) PageData {
	page := PageData{
		Title: "Срамная попка - итоги 2025 кускогода",
	}
	page.Nominations = append(page.Nominations, messagesTotal(msg))
	page.Nominations = append(page.Nominations, mostTotalUser(msg))
	page.Nominations = append(page.Nominations, minTotalUser(msg))
	page.Nominations = append(page.Nominations, firstMessage(msg))
	page.Nominations = append(page.Nominations, maxTikTok(msg))
	page.Nominations = append(page.Nominations, maxVideo(msg))
	page.Nominations = append(page.Nominations, maxPhotos(msg))
	page.Nominations = append(page.Nominations, longestWriter(msg))
	page.Nominations = append(page.Nominations, championByDays(msg))
	page.Nominations = append(page.Nominations, maxForward(msg))
	page.Nominations = append(page.Nominations, mostMentioned(msg))
	page.Nominations = append(page.Nominations, mostGivenReactions(msg))
	page.Nominations = append(page.Nominations, mostReactions(msg))
	page.Nominations = append(page.Nominations, emojiMaster(msg))
	page.Nominations = append(page.Nominations, mostUsedEmoji(msg))
	page.Nominations = append(page.Nominations, maxStickers(msg))
	page.Nominations = append(page.Nominations, maxDay(msg))

	return page
}

func main() {
	// Имя файла экспорта Telegram
	export, err := readFile("kuski.json")
	if err != nil {
		log.Fatal().Err(err).Msg("cannot read file")
	}

	messages := filterMessages(export.Messages, filterTypeMessage, filterYear(2025))

	// typ := map[string]struct{}{}
	// for _, m := range messages {
	// 	typ[m.MediaType] = struct{}{}
	// }
	// fmt.Println(typ)

	// // Печатаем первые 5 сообщений
	// for i, msg := range messages {
	// 	if i >= 5 {
	// 		break
	// 	}
	// 	fmt.Printf("\nMessage #%d\n", msg.ID)
	// 	fmt.Println("From:", msg.From)
	// 	fmt.Println("Date:", msg.Date)
	// 	fmt.Println("Text:", msg.Text)
	// }

	err = generateHTML("template_v7.html", "year_summary.html", formPage(messages))
	if err != nil {
		log.Fatal().Err(err).Msg("generate html")
	}
}
