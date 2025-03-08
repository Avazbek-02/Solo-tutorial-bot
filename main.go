package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xuri/excelize/v2"
)

// Tutorial tuzilishi
type Tutorial struct {
	Bio    string   `json:"bio"`
	Role   string   `json:"role"`
	Videos []string `json:"videos"`
}

// Qahramon tarixi tuzilishi
type Story struct {
	Bio    string   `json:"bio"`
	Role   string   `json:"role"`
	Videos []string `json:"videos"`
}

// Foydalanuvchi holat tuzilishi
type UserState struct {
	UserID   int64
	State    string
	TempData map[string]string
}

type BotData struct {
	Tutorials map[string]Tutorial  `json:"tutorials"`
	Admins    map[string]AdminInfo `json:"admins"`
	Stories   map[string]Story     `json:"stories"`
}

// Foydalanuvchi ma'lumotlari
type UserInfo struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// Foydalanuvchi harakati
type UserAction struct {
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
	Timestamp time.Time `json:"timestamp"`
}

const (
	// Holatlar
	STATE_NONE                   = "none"
	STATE_WAITING_TITLE          = "waiting_title"
	STATE_WAITING_BIO            = "waiting_bio"
	STATE_WAITING_ROLE           = "waiting_role"
	STATE_WAITING_VIDEO_ID       = "waiting_video_id"
	STATE_TUTORIAL_SELECTED      = "tutorial_selected"
	STATE_ADMIN_TUTORIAL_MENU    = "admin_tutorial_menu"
	STATE_CONFIRM_DELETE         = "confirm_delete"
	STATE_UPDATE_BIO             = "update_bio"
	STATE_ADD_VIDEO              = "add_video"
	STATE_ADD_ADMIN              = "add_admin"
	STATE_REMOVE_ADMIN           = "remove_admin"
	STATE_WAITING_STORY_TITLE    = "waiting_story_title"
	STATE_WAITING_STORY_BIO      = "waiting_story_bio"
	STATE_WAITING_STORY_ROLE     = "waiting_story_role"
	STATE_WAITING_STORY_VIDEO_ID = "waiting_story_video_id"
	STATE_STORY_SELECTED         = "story_selected"
	STATE_CONFIRM_DELETE_STORY   = "confirm_delete_story"
	STATE_UPDATE_STORY_BIO       = "update_story_bio"
	STATE_UPDATE_STORY_ROLE      = "update_story_role"
	STATE_ADD_STORY_VIDEO        = "add_story_video"
)

var (
	botToken       = "7294493521:AAFoWLQdo4-4On1Q3qNbbmXJn6Tljv1rDu8"
	adminUsername  = "D_Avazbek"      // @ belgisisiz
	privateChannel = "-1002377334931" // -100 bilan boshlanadi
	dataFile       = "tutorial_data.json"
	logsDir        = "user_logs"
	userStates     = make(map[int64]*UserState)
	userActions    = []UserAction{}
)

func main() {
	// Logs direktoryasini yaratish
	if err := os.MkdirAll(logsDir, os.ModePerm); err != nil {
		log.Printf("Logs papkasini yaratishda xatolik: %v", err)
	}

	// Bot yaratish
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Bot %s muvaffaqiyatli ishga tushdi!", bot.Self.UserName)

	// Yangilanishlarni qabul qilish uchun kanal
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updates := bot.GetUpdatesChan(updateConfig)

	// Yangilanishlarni qayta ishlash
	for update := range updates {
		// Xabar kelsa
		if update.Message != nil {
			handleMessage(bot, update.Message)
		} else if update.CallbackQuery != nil {
			handleCallbackQuery(bot, update.CallbackQuery)
		}
	}
}

// Callback query'larni qayta ishlash
func handleCallbackQuery(bot *tgbotapi.BotAPI, callbackQuery *tgbotapi.CallbackQuery) {
	// CallbackQuery javobini yuborish
	callback := tgbotapi.NewCallback(callbackQuery.ID, "")
	bot.Request(callback)

	// Foydalanuvchi holati
	userID := callbackQuery.From.ID
	state := getUserState(userID)
	chatID := callbackQuery.Message.Chat.ID

	// Ma'lumotlarni ajratish
	data := strings.Split(callbackQuery.Data, ":")
	action := data[0]

	// Harakatni qayd qilish
	logUserAction(callbackQuery.From, fmt.Sprintf("Callback: %s", action), "")

	switch action {
	case "delete_tutorial":
		if len(data) < 2 {
			return
		}
		tutorialTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		state.State = STATE_CONFIRM_DELETE
		state.TempData["deleteTitle"] = tutorialTitle

		// Tasdiqlash so'rovi
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Ha, o'chirish", fmt.Sprintf("confirm_delete:%s", tutorialTitle)),
				tgbotapi.NewInlineKeyboardButtonData("🔙 Bekor qilish", "cancel_delete"),
			),
		)

		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("'%s' bo'limini o'chirishni tasdiqlaysizmi?", tutorialTitle))
		msg.ReplyMarkup = keyboard
		bot.Send(msg)

	case "confirm_delete":
		if len(data) < 2 {
			return
		}
		tutorialTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		// Ma'lumotlarni yuklash
		botData := loadData()

		// Bo'limni o'chirish
		delete(botData.Tutorials, tutorialTitle)

		// Ma'lumotlarni saqlash
		saveData(botData)

		logUserAction(callbackQuery.From, "Admin: Bo'lim o'chirildi", tutorialTitle)
		sendMessage(bot, chatID, fmt.Sprintf("'%s' bo'limi muvaffaqiyatli o'chirildi.", tutorialTitle))
		sendAdminMenu(bot, chatID)
		resetUserState(userID)

	case "cancel_delete":
		sendAdminMenu(bot, chatID)
		resetUserState(userID)

	case "update_bio":
		if len(data) < 2 {
			return
		}
		tutorialTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		state.State = STATE_UPDATE_BIO
		state.TempData["updateTitle"] = tutorialTitle

		sendMessage(bot, chatID, fmt.Sprintf("'%s' bo'limi uchun yangi bio matnini kiriting:", tutorialTitle))

	case "add_video":
		if len(data) < 2 {
			return
		}
		tutorialTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		state.State = STATE_ADD_VIDEO
		state.TempData["updateTitle"] = tutorialTitle

		sendMessage(bot, chatID, fmt.Sprintf("'%s' bo'limi uchun qo'shiladigan video ID raqamini kiriting:", tutorialTitle))

	case "download_logs":
		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		// Excel faylni yaratib yuborish
		filePath, err := createExcelLog()
		if err != nil {
			sendMessage(bot, chatID, fmt.Sprintf("Excel faylini yaratishda xatolik: %v", err))
			return
		}

		// Faylni yuborish
		doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
		doc.Caption = "Foydalanuvchilar harakatlari jurnali"
		_, err = bot.Send(doc)
		if err != nil {
			sendMessage(bot, chatID, fmt.Sprintf("Excel faylini yuborishda xatolik: %v", err))
		}

		// Faylni o'chirish
		os.Remove(filePath)

	case "confirm_remove_admin":
		if len(data) < 2 {
			return
		}
		adminToRemove := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		// Adminni o'chirish
		removeAdmin(bot, chatID, adminToRemove, callbackQuery.From.UserName)
		// Adminlar ro'yxatini qayta ko'rsatish
		showAdminList(bot, chatID)
	case "add_admin":
		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		state.State = STATE_ADD_ADMIN
		sendMessage(bot, chatID, "Yangi admin username'ini kiriting (@username ko'rinishida):")

	case "remove_admin":
		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		// Ma'lumotlarni yuklash
		botData := loadData()

		if len(botData.Admins) == 0 {
			sendMessage(bot, chatID, "Qo'shimcha adminlar mavjud emas.")
			return
		}

		// Adminlarni ro'yxatdan o'chirish uchun inline tugmalar
		var rows [][]tgbotapi.InlineKeyboardButton
		for username := range botData.Admins {
			row := tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("@"+username, "confirm_remove_admin:"+username),
			)
			rows = append(rows, row)
		}

		keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
		msg := tgbotapi.NewMessage(chatID, "O'chirish uchun admin tanlang:")
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	case "update_role":
		if len(data) < 2 {
			return
		}
		tutorialTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		state.State = "update_role"
		state.TempData["tutorial"] = tutorialTitle

		// Role tanlash uchun tugmalar
		roleKeyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Marksman/ADK"),
				tgbotapi.NewKeyboardButton("Tank"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Fighter"),
				tgbotapi.NewKeyboardButton("Assassin"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Support"),
				tgbotapi.NewKeyboardButton("Mage"),
			),
		)
		roleKeyboard.ResizeKeyboard = true

		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Bo'lim '%s' uchun yangi rolni tanlang:", tutorialTitle))
		msg.ReplyMarkup = roleKeyboard
		bot.Send(msg)

	case "delete_story":
		if len(data) < 2 {
			return
		}
		storyTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		state.State = STATE_CONFIRM_DELETE_STORY
		state.TempData["deleteTitle"] = storyTitle

		// Tasdiqlash so'rovi
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Ha, o'chirish", fmt.Sprintf("confirm_delete_story:%s", storyTitle)),
				tgbotapi.NewInlineKeyboardButtonData("🔙 Bekor qilish", "cancel_delete_story"),
			),
		)

		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("'%s' geroy tarixini o'chirishni tasdiqlaysizmi?", storyTitle))
		msg.ReplyMarkup = keyboard
		bot.Send(msg)

	case "confirm_delete_story":
		if len(data) < 2 {
			return
		}
		storyTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		// Ma'lumotlarni yuklash
		botData := loadData()

		// Geroy tarixini o'chirish
		delete(botData.Stories, storyTitle)

		// Ma'lumotlarni saqlash
		saveData(botData)

		logUserAction(callbackQuery.From, "Admin: Geroy tarixi o'chirildi", storyTitle)
		sendMessage(bot, chatID, fmt.Sprintf("'%s' geroy tarixi muvaffaqiyatli o'chirildi.", storyTitle))
		sendAdminMenu(bot, chatID)
		resetUserState(userID)

	case "cancel_delete_story":
		sendAdminMenu(bot, chatID)
		resetUserState(userID)

	case "update_story_bio":
		if len(data) < 2 {
			return
		}
		storyTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		sendMessage(bot, chatID, fmt.Sprintf("'%s' geroy tarixi uchun yangi bio kiriting:", storyTitle))
		state.State = STATE_UPDATE_STORY_BIO
		state.TempData["updateTitle"] = storyTitle

	case "update_story_role":
		if len(data) < 2 {
			return
		}
		storyTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		// Role tanlash uchun tugmalar
		roleKeyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Marksman/ADK"),
				tgbotapi.NewKeyboardButton("Tank"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Fighter"),
				tgbotapi.NewKeyboardButton("Assassin"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Support"),
				tgbotapi.NewKeyboardButton("Mage"),
			),
		)
		roleKeyboard.ResizeKeyboard = true

		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("'%s' geroy tarixi uchun yangi rolni tanlang:", storyTitle))
		msg.ReplyMarkup = roleKeyboard
		bot.Send(msg)
		state.State = STATE_UPDATE_STORY_ROLE
		state.TempData["updateTitle"] = storyTitle

	case "add_story_video":
		if len(data) < 2 {
			return
		}
		storyTitle := data[1]

		// Faqat admin uchun
		if !isAdmin(callbackQuery.From.UserName) {
			sendMessage(bot, chatID, "Bu amal faqat adminlar uchun.")
			return
		}

		sendMessage(bot, chatID, fmt.Sprintf("'%s' geroy tarixi uchun yangi video ID raqamini kiriting:", storyTitle))
		state.State = STATE_ADD_STORY_VIDEO
		state.TempData["updateTitle"] = storyTitle
	}
}

// Xabarlarni qayta ishlash
func handleMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	// Foydalanuvchi holati
	userID := message.From.ID
	state := getUserState(userID)

	// Buyruqlarni tekshirish
	if message.IsCommand() {
		switch message.Command() {
		case "start":
			// Harakatni qayd qilish
			logUserAction(message.From, "Bot ishga tushirildi", "/start")

			// Admin uchun maxsus menyuni ko'rsatish
			if isAdmin(message.From.UserName) {
				sendAdminMenu(bot, message.Chat.ID)
			} else {
				sendMainMenu(bot, message.Chat.ID)
			}
			resetUserState(userID)
		case "create":
			// Faqat admin uchun
			if !isAdmin(message.From.UserName) {
				sendMessage(bot, message.Chat.ID, "Bu buyruq faqat admin uchun.")
				return
			}

			logUserAction(message.From, "Admin: Yangi bo'lim yaratish boshlandi", "/create")
			sendMessage(bot, message.Chat.ID, "Yangi bo'lim nomini kiriting:")
			state.State = STATE_WAITING_TITLE
		default:
			sendMessage(bot, message.Chat.ID, "Bunday buyruq mavjud emas.")
		}
		return
	}

	// Asosiy menyudagi tugmalarni tekshirish
	if message.Text == "Tutorials" {
		logUserAction(message.From, "Tutorials menyusiga kirdi", "")
		showTutorials(bot, message.Chat.ID)
		return
	} else if message.Text == "⬅️ Rollar" {
		logUserAction(message.From, "Rollar menyusiga qaytdi", "")
		if state.TempData["menu"] == "stories" {
			showStories(bot, message.Chat.ID)
		} else {
			showTutorials(bot, message.Chat.ID)
		}
		return
	} else if message.Text == "Geroylar tarixi" {
		logUserAction(message.From, "Geroylar tarixi menyusiga kirdi", "")
		showStories(bot, message.Chat.ID)
		return
	} else if message.Text == "⬅️ Orqaga" {
		logUserAction(message.From, "Orqaga qaytdi", "")

		// Qaysi menyuda ekanligini tekshirish
		if state.State == STATE_STORY_SELECTED {
			// Ma'lumotlarni yuklash
			data := loadData()

			// Agar geroy tarixi tanlangan bo'lsa, yana roliga qaytish
			if story, exists := data.Stories[state.TempData["selectedStory"]]; exists {
				showStoriesByRole(bot, message.Chat.ID, story.Role)
			} else {
				showStories(bot, message.Chat.ID)
			}
		} else if state.State == STATE_TUTORIAL_SELECTED {
			// Ma'lumotlarni yuklash
			data := loadData()

			// Agar tutorial tanlangan bo'lsa, yana roliga qaytish
			if tutorial, exists := data.Tutorials[state.TempData["selectedTutorial"]]; exists {
				showTutorialsByRole(bot, message.Chat.ID, tutorial.Role)
			} else {
				showTutorials(bot, message.Chat.ID)
			}
		} else {
			// Boshqa holatlarda asosiy menyuga qaytish
			if isAdmin(message.From.UserName) {
				sendAdminMenu(bot, message.Chat.ID)
			} else {
				sendMainMenu(bot, message.Chat.ID)
			}
		}
		resetUserState(userID)
		return
	} else if message.Text == "➕ Yangi bo'lim yaratish" && isAdmin(message.From.UserName) {
		// Admin yaratish tugmasini bosgan
		logUserAction(message.From, "Admin: Yangi bo'lim yaratish boshlandi", "")
		sendMessage(bot, message.Chat.ID, "Yangi bo'lim nomini kiriting:")
		state.State = STATE_WAITING_TITLE
		return
	} else if message.Text == "🔧 Bo'limlarni boshqarish" && isAdmin(message.From.UserName) {
		// Admin boshqarish tugmasini bosgan
		logUserAction(message.From, "Admin: Bo'limlarni boshqarish", "")
		showTutorialsForAdmin(bot, message.Chat.ID)
		return
	} else if message.Text == "📊 Statistika" && isAdmin(message.From.UserName) {
		// Admin statistika tugmasini bosgan
		logUserAction(message.From, "Admin: Statistikani so'radi", "")
		showStatistics(bot, message.Chat.ID)
		return
	} else if message.Text == "👥 Adminlar" && isAdmin(message.From.UserName) {
		// Admin adminlarni boshqarish tugmasini bosgan
		logUserAction(message.From, "Admin: Adminlar ro'yxatini so'radi", "")
		showAdminList(bot, message.Chat.ID)
		return
	} else if message.Text == "➕ Yangi geroy tarixi yaratish" && isAdmin(message.From.UserName) {
		// Admin yangi geroy tarixi yaratish tugmasini bosgan
		logUserAction(message.From, "Admin: Yangi geroy tarixi yaratish boshlandi", "")
		sendMessage(bot, message.Chat.ID, "Yangi geroy tarixi nomini kiriting:")
		state.State = STATE_WAITING_STORY_TITLE
		return
	} else if message.Text == "🔧 Geroylar tarixini boshqarish" && isAdmin(message.From.UserName) {
		// Admin geroylar tarixini boshqarish tugmasini bosgan
		logUserAction(message.From, "Admin: Geroylar tarixini boshqarish", "")
		showStoriesForAdmin(bot, message.Chat.ID)
		return
	}

	// HOLATGA QARAB XABARNI QAYTA ISHLASH - birinchi navbatda statelarni ko'rib chiqamiz
	switch state.State {
	case STATE_WAITING_TITLE:
		state.TempData["title"] = message.Text
		logUserAction(message.From, "Admin: Bo'lim nomi kiritildi", message.Text)
		sendMessage(bot, message.Chat.ID, fmt.Sprintf("Bo'lim '%s' uchun qisqacha tavsif (bio) kiriting:", message.Text))
		state.State = STATE_WAITING_BIO
		return

	case STATE_WAITING_BIO:
		state.TempData["bio"] = message.Text
		logUserAction(message.From, "Admin: Bo'lim uchun bio kiritildi", state.TempData["title"])

		// Role tanlash uchun tugmalar
		roleKeyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Marksman/ADK"),
				tgbotapi.NewKeyboardButton("Tank"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Fighter"),
				tgbotapi.NewKeyboardButton("Assassin"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Support"),
				tgbotapi.NewKeyboardButton("Mage"),
			),
		)
		roleKeyboard.ResizeKeyboard = true

		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Bo'lim '%s' uchun rolni tanlang:", state.TempData["title"]))
		msg.ReplyMarkup = roleKeyboard
		bot.Send(msg)
		state.State = STATE_WAITING_ROLE
		return

	case STATE_WAITING_ROLE:
		validRoles := []string{"Marksman/ADK", "Tank", "Fighter", "Assassin", "Support", "Mage"}
		isValidRole := false

		for _, role := range validRoles {
			if message.Text == role {
				isValidRole = true
				break
			}
		}

		if !isValidRole {
			sendMessage(bot, message.Chat.ID, "Noto'g'ri rol tanlandi. Iltimos, taqdim etilgan tugmalardan birini tanlang.")
			return
		}

		state.TempData["role"] = message.Text
		logUserAction(message.From, "Admin: Bo'lim uchun rol tanlandi", message.Text)
		sendMessage(bot, message.Chat.ID, "Endi video ID raqamini kiriting (private kanaldan):")
		state.State = STATE_WAITING_VIDEO_ID
		return

	case STATE_WAITING_VIDEO_ID:
		videoID := message.Text
		title := state.TempData["title"]
		bio := state.TempData["bio"]
		role := state.TempData["role"]

		// Ma'lumotlarni yuklash
		data := loadData()

		// Yangi bo'limni qo'shish
		if _, exists := data.Tutorials[title]; !exists {
			data.Tutorials[title] = Tutorial{
				Bio:    bio,
				Role:   role,
				Videos: []string{videoID},
			}
		} else {
			tutorial := data.Tutorials[title]
			tutorial.Videos = append(tutorial.Videos, videoID)
			// Agar o'zgartirilsa role ham yangilansin
			if tutorial.Role == "" {
				tutorial.Role = role
			}
			data.Tutorials[title] = tutorial
		}

		// Ma'lumotlarni saqlash
		saveData(data)

		logUserAction(message.From, "Admin: Yangi bo'lim yaratildi", title)
		sendMessage(bot, message.Chat.ID, fmt.Sprintf("Bo'lim '%s' muvaffaqiyatli yaratildi va video qo'shildi!", title))

		// Admin menyuga qaytish
		sendAdminMenu(bot, message.Chat.ID)
		resetUserState(userID)

	case STATE_UPDATE_BIO:
		title := state.TempData["updateTitle"]
		newBio := message.Text

		// Ma'lumotlarni yuklash
		data := loadData()

		// Bo'lim mavjudligini tekshirish
		if tutorial, exists := data.Tutorials[title]; exists {
			tutorial.Bio = newBio
			data.Tutorials[title] = tutorial
			saveData(data)
			logUserAction(message.From, "Admin: Bo'lim bio yangilandi", title)
			sendMessage(bot, message.Chat.ID, fmt.Sprintf("'%s' bo'limi uchun bio muvaffaqiyatli yangilandi!", title))
		} else {
			sendMessage(bot, message.Chat.ID, "Bo'lim topilmadi.")
		}

		sendAdminMenu(bot, message.Chat.ID)
		resetUserState(userID)

	case STATE_ADD_VIDEO:
		title := state.TempData["updateTitle"]
		videoID := message.Text

		// Ma'lumotlarni yuklash
		data := loadData()

		// Bo'lim mavjudligini tekshirish
		if tutorial, exists := data.Tutorials[title]; exists {
			tutorial.Videos = append(tutorial.Videos, videoID)
			data.Tutorials[title] = tutorial
			saveData(data)
			logUserAction(message.From, "Admin: Bo'limga video qo'shildi", title)
			sendMessage(bot, message.Chat.ID, fmt.Sprintf("'%s' bo'limi uchun yangi video muvaffaqiyatli qo'shildi!", title))
		} else {
			sendMessage(bot, message.Chat.ID, "Bo'lim topilmadi.")
		}

		sendAdminMenu(bot, message.Chat.ID)
		resetUserState(userID)
	case STATE_ADD_ADMIN:
		newAdminUsername := strings.TrimPrefix(message.Text, "@")

		// Faqat admin uchun
		if !isAdmin(message.From.UserName) {
			sendMessage(bot, message.Chat.ID, "Bu amal faqat adminlar uchun.")
			return
		}

		// O'zini o'zi qo'shishni tekshirish
		if newAdminUsername == message.From.UserName {
			sendMessage(bot, message.Chat.ID, "Siz allaqachon adminsiz.")
			resetUserState(userID)
			return
		}

		// Asosiy adminni qo'shishni tekshirish
		if newAdminUsername == adminUsername {
			sendMessage(bot, message.Chat.ID, "Bu foydalanuvchi asosiy admin.")
			resetUserState(userID)
			return
		}

		// Admin allaqachon mavjud ekanligini tekshirish
		data := loadData()
		if _, exists := data.Admins[newAdminUsername]; exists {
			sendMessage(bot, message.Chat.ID, fmt.Sprintf("@%s allaqachon admin ro'yxatida.", newAdminUsername))
			resetUserState(userID)
			return
		}

		// Yangi adminni qo'shish
		addAdmin(bot, message.Chat.ID, newAdminUsername, message.From.UserName)
		showAdminList(bot, message.Chat.ID)
		resetUserState(userID)
	case "update_role":
		validRoles := []string{"Marksman/ADK", "Tank", "Fighter", "Assassin", "Support", "Mage"}
		isValidRole := false

		for _, role := range validRoles {
			if message.Text == role {
				isValidRole = true
				break
			}
		}

		if !isValidRole {
			sendMessage(bot, message.Chat.ID, "Noto'g'ri rol tanlandi. Iltimos, taqdim etilgan tugmalardan birini tanlang.")
			return
		}

		tutorialTitle := state.TempData["tutorial"]
		newRole := message.Text

		// Ma'lumotlarni yuklash
		data := loadData()

		// Rolni yangilash
		if tutorial, exists := data.Tutorials[tutorialTitle]; exists {
			tutorial.Role = newRole
			data.Tutorials[tutorialTitle] = tutorial

			// Ma'lumotlarni saqlash
			saveData(data)

			logUserAction(message.From, "Admin: Bo'lim roli yangilandi", fmt.Sprintf("%s -> %s", tutorialTitle, newRole))
			sendMessage(bot, message.Chat.ID, fmt.Sprintf("Bo'lim '%s' uchun rol muvaffaqiyatli '%s' ga o'zgartirildi!", tutorialTitle, newRole))
		} else {
			sendMessage(bot, message.Chat.ID, "Bo'lim topilmadi.")
		}

		// Admin menyuga qaytish
		sendAdminMenu(bot, message.Chat.ID)
		resetUserState(userID)

	case STATE_WAITING_STORY_TITLE:
		state.TempData["title"] = message.Text
		logUserAction(message.From, "Admin: Geroy tarixi nomi kiritildi", message.Text)
		sendMessage(bot, message.Chat.ID, fmt.Sprintf("Geroy tarixi '%s' uchun qisqacha tavsif (bio) kiriting:", message.Text))
		state.State = STATE_WAITING_STORY_BIO
		return

	case STATE_WAITING_STORY_BIO:
		state.TempData["bio"] = message.Text
		logUserAction(message.From, "Admin: Geroy tarixi uchun bio kiritildi", state.TempData["title"])

		// Agar avval rol tanlangan bo'lsa (rol tanlagandan keyin yaratilayotgan bo'lsa)
		if selectedRole, exists := state.TempData["selectedRole"]; exists && selectedRole != "" {
			state.TempData["role"] = selectedRole
			logUserAction(message.From, "Admin: Geroy tarixi uchun rol avtomatik o'rnatildi", state.TempData["title"]+" -> "+selectedRole)
			sendMessage(bot, message.Chat.ID, fmt.Sprintf("Rol '%s' ga avtomatik o'rnatildi.\nEndi video ID raqamini kiriting (private kanaldan):", selectedRole))
			state.State = STATE_WAITING_STORY_VIDEO_ID
			return
		}

		// Avvalgi rol tanlanmagan bo'lsa, rolni so'rash
		roleKeyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Marksman/ADK"),
				tgbotapi.NewKeyboardButton("Tank"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Fighter"),
				tgbotapi.NewKeyboardButton("Assassin"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("Support"),
				tgbotapi.NewKeyboardButton("Mage"),
			),
		)
		roleKeyboard.ResizeKeyboard = true

		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Geroy tarixi '%s' uchun rolni tanlang:", state.TempData["title"]))
		msg.ReplyMarkup = roleKeyboard
		bot.Send(msg)
		state.State = STATE_WAITING_STORY_ROLE
		return

	case STATE_WAITING_STORY_ROLE:
		state.TempData["role"] = message.Text
		logUserAction(message.From, "Admin: Geroy tarixi uchun rol tanlandi", state.TempData["title"]+" -> "+message.Text)

		sendMessage(bot, message.Chat.ID, "Endi video ID raqamini kiriting (private kanaldan):")
		state.State = STATE_WAITING_STORY_VIDEO_ID
		return

	case STATE_WAITING_STORY_VIDEO_ID:
		videoID := message.Text
		title := state.TempData["title"]
		bio := state.TempData["bio"]
		role := state.TempData["role"]

		// Ma'lumotlarni yuklash
		data := loadData()

		// Yangi geroy tarixini qo'shish
		if _, exists := data.Stories[title]; !exists {
			data.Stories[title] = Story{
				Bio:    bio,
				Role:   role,
				Videos: []string{videoID},
			}
		} else {
			story := data.Stories[title]
			story.Videos = append(story.Videos, videoID)
			story.Bio = bio
			story.Role = role
			data.Stories[title] = story
		}

		// Ma'lumotlarni saqlash
		saveData(data)

		logUserAction(message.From, "Admin: Geroy tarixi yaratildi", title)
		sendMessage(bot, message.Chat.ID, fmt.Sprintf("Geroy tarixi '%s' muvaffaqiyatli yaratildi!", title))
		sendAdminMenu(bot, message.Chat.ID)
		resetUserState(userID)
		return

	case STATE_UPDATE_STORY_BIO:
		title := state.TempData["updateTitle"]
		newBio := message.Text

		// Ma'lumotlarni yuklash
		data := loadData()

		// Geroy tarixi mavjudligini tekshirish
		if story, exists := data.Stories[title]; exists {
			story.Bio = newBio
			data.Stories[title] = story
			saveData(data)
			logUserAction(message.From, "Admin: Geroy tarixi bio yangilandi", title)
			sendMessage(bot, message.Chat.ID, fmt.Sprintf("'%s' geroy tarixi uchun bio muvaffaqiyatli yangilandi!", title))
		} else {
			sendMessage(bot, message.Chat.ID, "Geroy tarixi topilmadi.")
		}

		sendAdminMenu(bot, message.Chat.ID)
		resetUserState(userID)
		return

	case STATE_UPDATE_STORY_ROLE:
		title := state.TempData["updateTitle"]
		newRole := message.Text

		validRoles := []string{"Marksman/ADK", "Tank", "Fighter", "Assassin", "Support", "Mage"}
		isValidRole := false

		for _, role := range validRoles {
			if newRole == role {
				isValidRole = true
				break
			}
		}

		if !isValidRole {
			sendMessage(bot, message.Chat.ID, "Noto'g'ri rol tanlandi. Iltimos, taqdim etilgan tugmalardan birini tanlang.")
			return
		}

		// Ma'lumotlarni yuklash
		data := loadData()

		// Geroy tarixi mavjudligini tekshirish
		if story, exists := data.Stories[title]; exists {
			story.Role = newRole
			data.Stories[title] = story
			saveData(data)
			logUserAction(message.From, "Admin: Geroy tarixi roli yangilandi", title)
			sendMessage(bot, message.Chat.ID, fmt.Sprintf("'%s' geroy tarixi uchun rol muvaffaqiyatli yangilandi!", title))
		} else {
			sendMessage(bot, message.Chat.ID, "Geroy tarixi topilmadi.")
		}

		sendAdminMenu(bot, message.Chat.ID)
		resetUserState(userID)
		return

	case STATE_ADD_STORY_VIDEO:
		title := state.TempData["updateTitle"]
		videoID := message.Text

		// Ma'lumotlarni yuklash
		data := loadData()

		// Geroy tarixi mavjudligini tekshirish
		if story, exists := data.Stories[title]; exists {
			story.Videos = append(story.Videos, videoID)
			data.Stories[title] = story
			saveData(data)
			logUserAction(message.From, "Admin: Geroy tarixiga video qo'shildi", title)
			sendMessage(bot, message.Chat.ID, fmt.Sprintf("'%s' geroy tarixiga yangi video muvaffaqiyatli qo'shildi!", title))
		} else {
			sendMessage(bot, message.Chat.ID, "Geroy tarixi topilmadi.")
		}

		sendAdminMenu(bot, message.Chat.ID)
		resetUserState(userID)
		return
	}

	// Rol tanlash
	validRoles := []string{"Marksman/ADK", "Tank", "Fighter", "Assassin", "Support", "Mage"}
	for _, role := range validRoles {
		if message.Text == role {
			if state.TempData["menu"] == "stories" {
				logUserAction(message.From, fmt.Sprintf("'%s' rolidagi Geroylar tarixini ko'rdi", role), "")
				showStoriesByRole(bot, message.Chat.ID, role)
			} else {
				logUserAction(message.From, fmt.Sprintf("'%s' rolidagi tutoriallarni ko'rdi", role), "")
				showTutorialsByRole(bot, message.Chat.ID, role)
			}
			return
		}
	}

	// Tutorial va Geroylar tarixi tanlash
	data := loadData()
	for title := range data.Tutorials {
		if message.Text == title {
			state.State = STATE_TUTORIAL_SELECTED
			state.TempData["selectedTutorial"] = title
			logUserAction(message.From, "Bo'lim tanlandi", title)
			showTutorialContent(bot, message.Chat.ID, title)
			return
		}
	}

	for title := range data.Stories {
		if message.Text == title {
			state.State = STATE_STORY_SELECTED
			state.TempData["selectedStory"] = title
			logUserAction(message.From, "Geroy tarixi tanlandi", title)
			showStoryContent(bot, message.Chat.ID, title)
			return
		}
	}
}

// Asosiy menyuni yuborish
func sendMainMenu(bot *tgbotapi.BotAPI, chatID int64) {
	// Oddiy klaviatura yaratish
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Tutorials"),
			tgbotapi.NewKeyboardButton("Geroylar tarixi"),
		),
	)

	// Klaviaturani sozlash
	keyboard.ResizeKeyboard = true
	keyboard.OneTimeKeyboard = false

	msg := tgbotapi.NewMessage(chatID, "Assalomu alaykum! Botimizga xush kelibsiz.")
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Admin menyusini yuborish
func sendAdminMenu(bot *tgbotapi.BotAPI, chatID int64) {
	// Admin uchun maxsus klaviatura
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Tutorials"),
			tgbotapi.NewKeyboardButton("Geroylar tarixi"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("➕ Yangi bo'lim yaratish"),
			tgbotapi.NewKeyboardButton("🔧 Bo'limlarni boshqarish"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("➕ Yangi geroy tarixi yaratish"),
			tgbotapi.NewKeyboardButton("🔧 Geroylar tarixini boshqarish"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Statistika"),
			tgbotapi.NewKeyboardButton("👥 Adminlar"),
		),
	)

	// Klaviaturani sozlash
	keyboard.ResizeKeyboard = true
	keyboard.OneTimeKeyboard = false

	msg := tgbotapi.NewMessage(chatID, "Admin panel")
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Mavjud bo'limlarni ko'rsatish
func showTutorials(bot *tgbotapi.BotAPI, chatID int64) {
	data := loadData()

	if len(data.Tutorials) == 0 {
		sendMessage(bot, chatID, "Hozircha tutorials mavjud emas.")
		sendMainMenu(bot, chatID)
		return
	}

	// Rol kategoriyalarini ko'rsatish uchun tugmalar yaratish
	roleKeyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Marksman/ADK"),
			tgbotapi.NewKeyboardButton("Tank"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Fighter"),
			tgbotapi.NewKeyboardButton("Assassin"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Support"),
			tgbotapi.NewKeyboardButton("Mage"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("⬅️ Orqaga"),
		),
	)
	roleKeyboard.ResizeKeyboard = true

	msg := tgbotapi.NewMessage(chatID, "Qaysi roldagi tutoriallarni ko'rmoqchisiz?")
	msg.ReplyMarkup = roleKeyboard
	bot.Send(msg)
}

// Rolga oid bo'limlarni ko'rsatish
func showTutorialsByRole(bot *tgbotapi.BotAPI, chatID int64, role string) {
	data := loadData()

	// Role bo'yicha tutoriallarni saralash
	var tutorialsInRole []string
	for title, tutorial := range data.Tutorials {
		if tutorial.Role == role {
			tutorialsInRole = append(tutorialsInRole, title)
		}
	}

	if len(tutorialsInRole) == 0 {
		sendMessage(bot, chatID, fmt.Sprintf("Hozircha '%s' rolida tutoriallar mavjud emas.", role))
		// Rollar menyusiga qaytish
		showTutorials(bot, chatID)
		return
	}

	// Bo'limlar uchun tugmalar yaratish
	var rows [][]tgbotapi.KeyboardButton

	// Har bir bo'lim uchun tugma yaratish
	for _, title := range tutorialsInRole {
		row := tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(title))
		rows = append(rows, row)
	}

	// Orqaga qaytish tugmasi
	backRow := tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("⬅️ Rollar"))
	rows = append(rows, backRow)

	keyboard := tgbotapi.NewReplyKeyboard(rows...)
	keyboard.ResizeKeyboard = true

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("'%s' rolidagi mavjud tutoriallar:", role))
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Admin uchun bo'limlarni boshqarish menyusini ko'rsatish
func showTutorialsForAdmin(bot *tgbotapi.BotAPI, chatID int64) {
	data := loadData()

	if len(data.Tutorials) == 0 {
		sendMessage(bot, chatID, "Hozircha tutorials mavjud emas.")
		sendAdminMenu(bot, chatID)
		return
	}

	sendMessage(bot, chatID, "Quyidagi bo'limlar mavjud. Boshqarish uchun bo'limni tanlang:")

	// Har bir bo'lim uchun inline tugmalar yaratish
	for title, tutorial := range data.Tutorials {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✏️ Bio o'zgartirish", fmt.Sprintf("update_bio:%s", title)),
				tgbotapi.NewInlineKeyboardButtonData("🎮 Rol o'zgartirish", fmt.Sprintf("update_role:%s", title)),
				tgbotapi.NewInlineKeyboardButtonData("🎬 Video qo'shish", fmt.Sprintf("add_video:%s", title)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ O'chirish", fmt.Sprintf("delete_tutorial:%s", title)),
			),
		)

		roleInfo := ""
		if tutorial.Role != "" {
			roleInfo = fmt.Sprintf(" | Rol: %s", tutorial.Role)
		}

		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("📚 %s%s", title, roleInfo))
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}

	// Orqaga qaytish klaviaturasini yuborish
	backKeyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("⬅️ Orqaga"),
		),
	)
	backKeyboard.ResizeKeyboard = true

	msg := tgbotapi.NewMessage(chatID, "Orqaga qaytish uchun tugmani bosing.")
	msg.ReplyMarkup = backKeyboard
	bot.Send(msg)
}

// Bo'lim tarkibini ko'rsatish
func showTutorialContent(bot *tgbotapi.BotAPI, chatID int64, tutorialTitle string) {
	data := loadData()

	if tutorial, exists := data.Tutorials[tutorialTitle]; exists {
		// Har bir video uchun
		for i, videoID := range tutorial.Videos {
			// Agar bu birinchi video bo'lsa, bo'lim ma'lumotini video bilan birgalikda yuboramiz
			if i == 0 {
				// Videoni yuborish - CopyMessage orqali
				copyMsg := tgbotapi.NewCopyMessage(chatID,
					parseChannelID(privateChannel),
					getMessageID(videoID))

				// Video bilan birga caption qo'shamiz
				roleInfo := ""
				if tutorial.Role != "" {
					roleInfo = fmt.Sprintf("\n🎮 Rol: %s", tutorial.Role)
				}

				copyMsg.Caption = fmt.Sprintf("📚 %s%s\n\n%s", tutorialTitle, roleInfo, tutorial.Bio)

				_, err := bot.CopyMessage(copyMsg)
				if err != nil {
					log.Printf("Video yuborishda xatolik: %v", err)
					sendMessage(bot, chatID, fmt.Sprintf("Video yuborishda xatolik yuz berdi. ID: %s. Xatolik: %v", videoID, err))

					// Xatolik yuz berganda alternativ usul - forward qilishni sinab ko'rish
					forwardMsg := tgbotapi.NewForward(chatID,
						parseChannelID(privateChannel),
						getMessageID(videoID))

					_, forwardErr := bot.Send(forwardMsg)
					if forwardErr != nil {
						log.Printf("Forward qilishda ham xatolik: %v", forwardErr)
					}
				}
			} else {
				// Qolgan videolar uchun odatiy ko'rinishda yuboramiz
				copyMsg := tgbotapi.NewCopyMessage(chatID,
					parseChannelID(privateChannel),
					getMessageID(videoID))

				_, err := bot.CopyMessage(copyMsg)
				if err != nil {
					log.Printf("Video yuborishda xatolik: %v", err)
					sendMessage(bot, chatID, fmt.Sprintf("Video yuborishda xatolik yuz berdi. ID: %s. Xatolik: %v", videoID, err))

					// Xatolik yuz berganda alternativ usul - forward qilishni sinab ko'rish
					forwardMsg := tgbotapi.NewForward(chatID,
						parseChannelID(privateChannel),
						getMessageID(videoID))

					_, forwardErr := bot.Send(forwardMsg)
					if forwardErr != nil {
						log.Printf("Forward qilishda ham xatolik: %v", forwardErr)
					}
				}
			}
		}

		// Agar hech qanday video bo'lmasa, faqat ma'lumotni yuboramiz
		if len(tutorial.Videos) == 0 {
			sendMessage(bot, chatID, fmt.Sprintf("📚 %s\n\n%s\n\nBu bo'limda hali videolar mavjud emas.", tutorialTitle, tutorial.Bio))
		}

		// Orqaga qaytish klaviaturasini yuborish
		backKeyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("⬅️ Orqaga"),
			),
		)
		backKeyboard.ResizeKeyboard = true

		msg := tgbotapi.NewMessage(chatID, "Orqaga qaytish uchun tugmani bosing.")
		msg.ReplyMarkup = backKeyboard
		bot.Send(msg)
	} else {
		sendMessage(bot, chatID, "Bunday bo'lim topilmadi.")
		sendMainMenu(bot, chatID)
	}
}

// Statistikani ko'rsatish
func showStatistics(bot *tgbotapi.BotAPI, chatID int64) {
	// Faqat admin uchun
	if !isAdmin(getUsernameByID(chatID)) {
		sendMessage(bot, chatID, "Bu ma'lumot faqat admin uchun.")
		return
	}

	// Hisobotni yuborish uchun inline tugma
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📥 Excel hisobotini yuklash", "download_logs"),
		),
	)

	// Statistika ma'lumotlarini to'plash
	data := loadData()
	uniqueUsers := make(map[int64]bool)

	// Foydalanuvchilar sonini hisoblash
	for _, action := range userActions {
		uniqueUsers[action.UserID] = true
	}

	// Top ko'rilgan bo'limlar
	viewedTutorials := make(map[string]int)
	for _, action := range userActions {
		if action.Action == "Bo'lim ko'rildi" {
			viewedTutorials[action.Details]++
		}
	}

	// Statistika matnini yaratish
	statsText := fmt.Sprintf("📊 Bot statistikasi:\n\n"+
		"• Jami foydalanuvchilar: %d\n"+
		"• Jami bo'limlar: %d\n"+
		"• Jami harakatlar: %d\n\n",
		len(uniqueUsers), len(data.Tutorials), len(userActions))

	// Eng ko'p ko'rilgan bo'limlar
	statsText += "🔝 Eng ko'p ko'rilgan bo'limlar:\n"
	count := 0
	for tutorial, views := range viewedTutorials {
		if count < 5 { // Eng ko'p ko'rilgan 5 ta bo'lim
			statsText += fmt.Sprintf("%d. %s - %d marta\n", count+1, tutorial, views)
			count++
		} else {
			break
		}
	}

	// Statistika ma'lumotini yuborish
	msg := tgbotapi.NewMessage(chatID, statsText)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Excel hisobot yaratish
func createExcelLog() (string, error) {
	f := excelize.NewFile()

	// Yangi sheet yaratish
	sheetName := "Foydalanuvchilar"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return "", err
	}

	// Sarlavhalarni qo'shish
	f.SetCellValue(sheetName, "A1", "ID")
	f.SetCellValue(sheetName, "B1", "Username")
	f.SetCellValue(sheetName, "C1", "Ism")
	f.SetCellValue(sheetName, "D1", "Familiya")
	f.SetCellValue(sheetName, "E1", "Harakat")
	f.SetCellValue(sheetName, "F1", "Tafsilotlar")
	f.SetCellValue(sheetName, "G1", "Vaqt")

	// Sarlavhalarni formatlash
	// Sarlavhalarni formatlash
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#DCE6F1"},
			Pattern: 1,
		},
	})
	if err != nil {
		return "", err
	}
	f.SetCellStyle(sheetName, "A1", "G1", style)

	// Ma'lumotlarni qo'shish
	for i, action := range userActions {
		row := i + 2 // 1-qator sarlavha
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), action.UserID)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), action.Username)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), action.FirstName)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), action.LastName)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), action.Action)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), action.Details)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), action.Timestamp.Format("2006-01-02 15:04:05"))
	}

	// Ustunlarni kenglashtirish
	f.SetColWidth(sheetName, "A", "A", 15)
	f.SetColWidth(sheetName, "B", "B", 20)
	f.SetColWidth(sheetName, "C", "C", 20)
	f.SetColWidth(sheetName, "D", "D", 20)
	f.SetColWidth(sheetName, "E", "E", 25)
	f.SetColWidth(sheetName, "F", "F", 30)
	f.SetColWidth(sheetName, "G", "G", 20)

	// Default sheet o'rnatish
	f.SetActiveSheet(index)

	// Faylni saqlash
	fileName := fmt.Sprintf("user_logs_%s.xlsx", time.Now().Format("2006-01-02_15-04-05"))
	filePath := filepath.Join(logsDir, fileName)
	if err := f.SaveAs(filePath); err != nil {
		return "", err
	}

	return filePath, nil
}

// Ma'lumotlarni yuklash
func loadData() BotData {
	var data BotData

	// Fayl mavjudligini tekshirish
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		// Fayl mavjud bo'lmasa, bo'sh ma'lumotlar tuzilmasini qaytarish
		return BotData{
			Tutorials: make(map[string]Tutorial),
			Admins:    make(map[string]AdminInfo),
			Stories:   make(map[string]Story),
		}
	}

	// Faylni o'qish
	fileData, err := ioutil.ReadFile(dataFile)
	if err != nil {
		log.Printf("Faylni o'qishda xatolik: %v", err)
		return BotData{
			Tutorials: make(map[string]Tutorial),
			Admins:    make(map[string]AdminInfo),
			Stories:   make(map[string]Story),
		}
	}

	// JSON formatdan dekodlash
	err = json.Unmarshal(fileData, &data)
	if err != nil {
		log.Printf("JSON dekodlashda xatolik: %v", err)
		return BotData{
			Tutorials: make(map[string]Tutorial),
			Admins:    make(map[string]AdminInfo),
			Stories:   make(map[string]Story),
		}
	}

	// Adminlar map'ini yaratish (agar mavjud bo'lmasa)
	if data.Admins == nil {
		data.Admins = make(map[string]AdminInfo)
	}

	return data
}

// Ma'lumotlarni saqlash
func saveData(data BotData) {
	// JSON formatga kodlash
	fileData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("JSON kodlashda xatolik: %v", err)
		return
	}

	// Faylga yozish
	err = ioutil.WriteFile(dataFile, fileData, 0644)
	if err != nil {
		log.Printf("Faylga yozishda xatolik: %v", err)
	}
}

// Foydalanuvchi holatini olish
func getUserState(userID int64) *UserState {
	if state, exists := userStates[userID]; exists {
		return state
	}

	// Yangi holat yaratish
	state := &UserState{
		UserID:   userID,
		State:    STATE_NONE,
		TempData: make(map[string]string),
	}
	userStates[userID] = state
	return state
}

// Foydalanuvchi holatini tiklash
func resetUserState(userID int64) {
	if state, exists := userStates[userID]; exists {
		state.State = STATE_NONE
		state.TempData = make(map[string]string)
	}
}

// Xabar yuborish
func sendMessage(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	bot.Send(msg)
}

// Foydalanuvchi harakatini qayd qilish
func logUserAction(user *tgbotapi.User, action, details string) {
	// Yangi harakat yaratish
	newAction := UserAction{
		UserID:    user.ID,
		Username:  user.UserName,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Action:    action,
		Details:   details,
		Timestamp: time.Now(),
	}

	// Harakatni massivga qo'shish
	userActions = append(userActions, newAction)

	// Log faylini yaratish
	fileName := fmt.Sprintf("%s/actions_%s.log", logsDir, time.Now().Format("2006-01-02"))
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Log faylini yaratishda xatolik: %v", err)
		return
	}
	defer file.Close()

	// Harakatni log fayliga yozish
	actionLog := fmt.Sprintf("[%s] User: %s (ID: %d), Action: %s, Details: %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		user.UserName, user.ID, action, details)
	if _, err := file.WriteString(actionLog); err != nil {
		log.Printf("Log fayliga yozishda xatolik: %v", err)
	}
}

// Foydalanuvchi nomini ID bo'yicha olish
func getUsernameByID(userID int64) string {
	for _, action := range userActions {
		if action.UserID == userID {
			return action.Username
		}
	}
	return ""
}

// Kanal ID sini to'g'ri formatga keltirish
func parseChannelID(channelID string) int64 {
	// Agar ID "-100" bilan boshlansa, raqamga o'tkazish
	id, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		log.Printf("Kanal ID ni o'zgartirishda xatolik: %v", err)
		return 0
	}
	return id
}

// Message ID ni tekshirish va o'zgartirish
func getMessageID(videoID string) int {
	// Video ID ni int ga o'tkazish
	id, err := strconv.Atoi(videoID)
	if err != nil {
		log.Printf("Video ID ni o'zgartirishda xatolik: %v", err)
		return 0
	}
	return id
}

// Admin ma'lumotlari tuzilishi
type AdminInfo struct {
	Username string    `json:"username"`
	AddedBy  string    `json:"added_by"`
	AddedAt  time.Time `json:"added_at"`
}

// Foydalanuvchi admin ekanligini tekshirish
func isAdmin(username string) bool {
	if username == "" {
		return false
	}

	// Asosiy admin har doim admin hisoblanadi
	if username == adminUsername {
		return true
	}

	// Ma'lumotlarni yuklash
	data := loadData()

	// Adminlar ro'yxatida tekshirish
	_, isAdmin := data.Admins[username]

	return isAdmin
}

// Admin qo'shish
func addAdmin(bot *tgbotapi.BotAPI, chatID int64, newAdminUsername string, addedBy string) {
	// Ma'lumotlarni yuklash
	data := loadData()

	// Adminlar map'ini yaratish (agar mavjud bo'lmasa)
	if data.Admins == nil {
		data.Admins = make(map[string]AdminInfo)
	}

	// @ belgisini olib tashlash (agar bo'lsa)
	newAdminUsername = strings.TrimPrefix(newAdminUsername, "@")

	// Adminni qo'shish
	data.Admins[newAdminUsername] = AdminInfo{
		Username: newAdminUsername,
		AddedBy:  addedBy,
		AddedAt:  time.Now(),
	}

	// Ma'lumotlarni saqlash
	saveData(data)

	logUserAction(&tgbotapi.User{UserName: addedBy}, "Admin qo'shildi", newAdminUsername)
	sendMessage(bot, chatID, fmt.Sprintf("✅ @%s adminlar ro'yxatiga qo'shildi.", newAdminUsername))
}

// Adminni o'chirish
func removeAdmin(bot *tgbotapi.BotAPI, chatID int64, targetAdmin string, removedBy string) {
	// Asosiy adminni o'chirib bo'lmaydi
	if targetAdmin == adminUsername {
		sendMessage(bot, chatID, "❌ Asosiy adminni o'chirib bo'lmaydi.")
		return
	}

	// Ma'lumotlarni yuklash
	data := loadData()

	// @ belgisini olib tashlash (agar bo'lsa)
	if strings.HasPrefix(targetAdmin, "@") {
		targetAdmin = targetAdmin[1:]
	}

	// Adminlar ro'yxatida tekshirish
	if _, exists := data.Admins[targetAdmin]; !exists {
		sendMessage(bot, chatID, "❌ Bu foydalanuvchi adminlar ro'yxatida yo'q.")
		return
	}

	// Adminlar ro'yxatidan o'chirish
	delete(data.Admins, targetAdmin)

	// O'zgarishlarni saqlash
	saveData(data)

	// Harakatni qayd qilish
	logUserAction(&tgbotapi.User{UserName: removedBy}, "Admin o'chirildi", targetAdmin)
	sendMessage(bot, chatID, fmt.Sprintf("✅ @%s adminlar ro'yxatidan o'chirildi.", targetAdmin))
}

// Adminlar ro'yxatini ko'rsatish
func showAdminList(bot *tgbotapi.BotAPI, chatID int64) {
	// Ma'lumotlarni yuklash
	data := loadData()

	// Adminlar ro'yxatini yaratish
	adminList := fmt.Sprintf("👤 Asosiy admin: @%s\n\n", adminUsername)
	adminList += "📋 Qo'shimcha adminlar:\n"

	if len(data.Admins) == 0 {
		adminList += "Qo'shimcha adminlar mavjud emas."
	} else {
		i := 1
		for username, info := range data.Admins {
			adminList += fmt.Sprintf("%d. @%s (Qo'shgan: @%s, Vaqti: %s)\n",
				i, username, info.AddedBy, info.AddedAt.Format("2006-01-02 15:04:05"))
			i++
		}
	}

	// Adminlarni boshqarish uchun inline tugmalar
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Admin qo'shish", "add_admin"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Admin o'chirish", "remove_admin"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, adminList)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Admin uchun Geroylar tarixini boshqarish menyusini ko'rsatish
func showStoriesForAdmin(bot *tgbotapi.BotAPI, chatID int64) {
	data := loadData()

	if len(data.Stories) == 0 {
		sendMessage(bot, chatID, "Hozircha Geroylar tarixi mavjud emas.")
		sendAdminMenu(bot, chatID)
		return
	}

	sendMessage(bot, chatID, "Quyidagi Geroylar tarixi mavjud. Boshqarish uchun tarixni tanlang:")

	// Har bir tarix uchun inline tugmalar yaratish
	for title, story := range data.Stories {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✏️ Bio o'zgartirish", fmt.Sprintf("update_story_bio:%s", title)),
				tgbotapi.NewInlineKeyboardButtonData("🎮 Rol o'zgartirish", fmt.Sprintf("update_story_role:%s", title)),
				tgbotapi.NewInlineKeyboardButtonData("🎬 Video qo'shish", fmt.Sprintf("add_story_video:%s", title)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ O'chirish", fmt.Sprintf("delete_story:%s", title)),
			),
		)

		roleInfo := ""
		if story.Role != "" {
			roleInfo = fmt.Sprintf(" | Rol: %s", story.Role)
		}

		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("📖 %s%s", title, roleInfo))
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}

	// Orqaga qaytish klaviaturasini yuborish
	backKeyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("⬅️ Orqaga"),
		),
	)
	backKeyboard.ResizeKeyboard = true

	msg := tgbotapi.NewMessage(chatID, "Orqaga qaytish uchun tugmani bosing.")
	msg.ReplyMarkup = backKeyboard
	bot.Send(msg)
}

// Mavjud Geroylar tarixini ko'rsatish
func showStories(bot *tgbotapi.BotAPI, chatID int64) {
	data := loadData()

	// Foydalanuvchi holatini o'rnatish - stories menyu
	// Set the menu state for all users associated with this chat
	for userID := range userStates {
		state := userStates[userID]
		if state.UserID == chatID || userID == chatID {
			state.TempData["menu"] = "stories"
			userStates[userID] = state
		}
	}

	// If this is a new user/chat, create a state
	if _, exists := userStates[chatID]; !exists {
		userStates[chatID] = &UserState{
			UserID:   chatID,
			State:    "",
			TempData: map[string]string{"menu": "stories"},
		}
	}

	// Rol kategoriyalarini ko'rsatish uchun tugmalar yaratish
	roleKeyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Marksman/ADK"),
			tgbotapi.NewKeyboardButton("Tank"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Fighter"),
			tgbotapi.NewKeyboardButton("Assassin"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Support"),
			tgbotapi.NewKeyboardButton("Mage"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("⬅️ Orqaga"),
		),
	)
	roleKeyboard.ResizeKeyboard = true

	// Xabarni tanlash - Geroylar tarixi bo'lsa/bo'lmasa
	var message string
	if len(data.Stories) == 0 {
		message = "Hozircha Geroylar tarixi mavjud emas. Lekin siz rol tanlab ko'rishingiz mumkin."
	} else {
		message = "Qaysi roldagi Geroylar tarixini ko'rmoqchisiz?"
	}

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ReplyMarkup = roleKeyboard
	bot.Send(msg)
}

// Rolga oid Geroylar tarixini ko'rsatish
func showStoriesByRole(bot *tgbotapi.BotAPI, chatID int64, role string) {
	data := loadData()

	// Role bo'yicha geroylar tarixini saralash
	var storiesInRole []string
	for title, story := range data.Stories {
		if story.Role == role {
			storiesInRole = append(storiesInRole, title)
		}
	}

	// Tanlangan rolni saqlash - keyin geroy yaratishda kerak bo'lishi mumkin
	for userID := range userStates {
		state := userStates[userID]
		if state.UserID == chatID || userID == chatID {
			state.TempData["selectedRole"] = role
			userStates[userID] = state
		}
	}

	// If this is a new user/chat, create a state
	if _, exists := userStates[chatID]; !exists {
		userStates[chatID] = &UserState{
			UserID:   chatID,
			State:    "",
			TempData: map[string]string{"menu": "stories", "selectedRole": role},
		}
	}

	// Geroylar tarixi uchun tugmalar yaratish
	var rows [][]tgbotapi.KeyboardButton

	if len(storiesInRole) == 0 {
		sendMessage(bot, chatID, fmt.Sprintf("Hozircha '%s' rolida Geroylar tarixi mavjud emas.", role))

		// Admin uchun agar mavjud bo'lmasa, yangi yaratish ni taklif qilish
		if isAdmin(getUsernameByID(chatID)) {
			// Yangi yaratish yoki orqaga qaytish uchun tugmalar
			rows = append(rows, tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("➕ Yangi geroy tarixi yaratish")))
		}
	} else {
		// Har bir tarix uchun tugma yaratish
		for _, title := range storiesInRole {
			row := tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(title))
			rows = append(rows, row)
		}
	}

	// Orqaga qaytish tugmasi
	backRow := tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("⬅️ Rollar"))
	rows = append(rows, backRow)

	keyboard := tgbotapi.NewReplyKeyboard(rows...)
	keyboard.ResizeKeyboard = true

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("'%s' rolidagi Geroylar tarixini tanlang:", role))
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Geroy tarixi tarkibini ko'rsatish
func showStoryContent(bot *tgbotapi.BotAPI, chatID int64, storyTitle string) {
	data := loadData()

	if story, exists := data.Stories[storyTitle]; exists {
		// Har bir video uchun
		for i, videoID := range story.Videos {
			// Agar bu birinchi video bo'lsa, tarix ma'lumotini video bilan birgalikda yuboramiz
			if i == 0 {
				// Videoni yuborish - CopyMessage orqali
				copyMsg := tgbotapi.NewCopyMessage(chatID,
					parseChannelID(privateChannel),
					getMessageID(videoID))

				// Video bilan birga caption qo'shamiz
				roleInfo := ""
				if story.Role != "" {
					roleInfo = fmt.Sprintf("\n🎮 Rol: %s", story.Role)
				}

				copyMsg.Caption = fmt.Sprintf("📖 %s%s\n\n%s", storyTitle, roleInfo, story.Bio)

				_, err := bot.CopyMessage(copyMsg)
				if err != nil {
					log.Printf("Video yuborishda xatolik: %v", err)
					sendMessage(bot, chatID, fmt.Sprintf("Video yuborishda xatolik yuz berdi. ID: %s. Xatolik: %v", videoID, err))

					// Xatolik yuz berganda alternativ usul - forward qilishni sinab ko'rish
					forwardMsg := tgbotapi.NewForward(chatID,
						parseChannelID(privateChannel),
						getMessageID(videoID))

					_, forwardErr := bot.Send(forwardMsg)
					if forwardErr != nil {
						log.Printf("Forward qilishda ham xatolik: %v", forwardErr)
					}
				}
			} else {
				// Qolgan videolar uchun oddiy videolarni yuborish
				forwardMsg := tgbotapi.NewForward(chatID,
					parseChannelID(privateChannel),
					getMessageID(videoID))
				bot.Send(forwardMsg)
			}
		}

		// Orqaga qaytish tugmasi
		backKeyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("⬅️ Orqaga")),
		)
		backKeyboard.ResizeKeyboard = true

		msg := tgbotapi.NewMessage(chatID, "Orqaga qaytish uchun tugmani bosing.")
		msg.ReplyMarkup = backKeyboard
		bot.Send(msg)
	} else {
		sendMessage(bot, chatID, "Bunday Geroylar tarixi mavjud emas.")
		sendMainMenu(bot, chatID)
	}
}
