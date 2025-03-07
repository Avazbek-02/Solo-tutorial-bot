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
	STATE_NONE                = "none"
	STATE_WAITING_TITLE       = "waiting_title"
	STATE_WAITING_BIO         = "waiting_bio"
	STATE_WAITING_VIDEO_ID    = "waiting_video_id"
	STATE_TUTORIAL_SELECTED   = "tutorial_selected"
	STATE_ADMIN_TUTORIAL_MENU = "admin_tutorial_menu"
	STATE_CONFIRM_DELETE      = "confirm_delete"
	STATE_UPDATE_BIO          = "update_bio"
	STATE_ADD_VIDEO           = "add_video"
	STATE_ADD_ADMIN           = "add_admin"
	STATE_REMOVE_ADMIN        = "remove_admin"
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
	} else if message.Text == "Storys" {
		logUserAction(message.From, "Storys menyusiga kirdi", "")
		sendMessage(bot, message.Chat.ID, "Bu bo'lim hozircha mavjud emas.")
		if isAdmin(message.From.UserName) {
			sendAdminMenu(bot, message.Chat.ID)
		} else {
			sendMainMenu(bot, message.Chat.ID)
		}
		return
	} else if message.Text == "⬅️ Orqaga" {
		logUserAction(message.From, "Asosiy menyuga qaytdi", "")
		// Admin uchun maxsus menyuni ko'rsatish
		if isAdmin(message.From.UserName) {
			sendAdminMenu(bot, message.Chat.ID)
		} else {
			sendMainMenu(bot, message.Chat.ID)
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
	}

	// Bo'lim tanlangan bo'lsa
	if state.State == STATE_TUTORIAL_SELECTED {
		// Bo'lim tanlanganda
		tutorialTitle := state.TempData["selectedTutorial"]
		logUserAction(message.From, "Bo'lim ko'rildi", tutorialTitle)
		showTutorialContent(bot, message.Chat.ID, tutorialTitle)
		resetUserState(userID)
		return
	}

	// Bo'limlar ro'yxatini tekshirish
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

	// Holatga qarab xabarni qayta ishlash
	switch state.State {
	case STATE_WAITING_TITLE:
		state.TempData["title"] = message.Text
		logUserAction(message.From, "Admin: Bo'lim nomi kiritildi", message.Text)
		sendMessage(bot, message.Chat.ID, fmt.Sprintf("Bo'lim '%s' uchun qisqacha tavsif (bio) kiriting:", message.Text))
		state.State = STATE_WAITING_BIO

	case STATE_WAITING_BIO:
		state.TempData["bio"] = message.Text
		logUserAction(message.From, "Admin: Bo'lim uchun bio kiritildi", state.TempData["title"])
		sendMessage(bot, message.Chat.ID, "Endi video ID raqamini kiriting (private kanaldan):")
		state.State = STATE_WAITING_VIDEO_ID

	case STATE_WAITING_VIDEO_ID:
		videoID := message.Text
		title := state.TempData["title"]
		bio := state.TempData["bio"]

		// Ma'lumotlarni yuklash
		data := loadData()

		// Yangi bo'limni qo'shish
		if _, exists := data.Tutorials[title]; !exists {
			data.Tutorials[title] = Tutorial{
				Bio:    bio,
				Videos: []string{videoID},
			}
		} else {
			tutorial := data.Tutorials[title]
			tutorial.Videos = append(tutorial.Videos, videoID)
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
	}
}

// Asosiy menyuni yuborish
func sendMainMenu(bot *tgbotapi.BotAPI, chatID int64) {
	// Oddiy klaviatura yaratish
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Tutorials"),
			tgbotapi.NewKeyboardButton("Storys"),
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
			tgbotapi.NewKeyboardButton("Storys"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("➕ Yangi bo'lim yaratish"),
			tgbotapi.NewKeyboardButton("🔧 Bo'limlarni boshqarish"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Statistika"),
			tgbotapi.NewKeyboardButton("👥 Adminlar"),
		),
	)

	// Klaviaturani sozlash
	keyboard.ResizeKeyboard = true
	keyboard.OneTimeKeyboard = false

	msg := tgbotapi.NewMessage(chatID, "Admin paneli. Kerakli bo'limni tanlang:")
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

	// Bo'limlar uchun tugmalar yaratish
	var rows [][]tgbotapi.KeyboardButton

	// Har bir bo'lim uchun tugma yaratish
	for title := range data.Tutorials {
		row := tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(title))
		rows = append(rows, row)
	}

	// Orqaga qaytish tugmasi
	backRow := tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("⬅️ Orqaga"))
	rows = append(rows, backRow)

	keyboard := tgbotapi.NewReplyKeyboard(rows...)
	keyboard.ResizeKeyboard = true

	msg := tgbotapi.NewMessage(chatID, "Mavjud bo'limlar:")
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
	for title := range data.Tutorials {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✏️ Bio o'zgartirish", fmt.Sprintf("update_bio:%s", title)),
				tgbotapi.NewInlineKeyboardButtonData("🎬 Video qo'shish", fmt.Sprintf("add_video:%s", title)),
				tgbotapi.NewInlineKeyboardButtonData("❌ O'chirish", fmt.Sprintf("delete_tutorial:%s", title)),
			),
		)

		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("📚 %s", title))
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
		// Bo'lim haqida ma'lumot yuborish - buni endi videolar bilan birgalikda yuboramiz
		// sendMessage(bot, chatID, fmt.Sprintf("📚 %s\n\n%s", tutorialTitle, tutorial.Bio))

		// Har bir video uchun
		for i, videoID := range tutorial.Videos {
			// Agar bu birinchi video bo'lsa, bo'lim ma'lumotini video bilan birgalikda yuboramiz
			if i == 0 {
				// Videoni yuborish - CopyMessage orqali
				copyMsg := tgbotapi.NewCopyMessage(chatID,
					parseChannelID(privateChannel),
					getMessageID(videoID))

				// Video bilan birga caption qo'shamiz
				copyMsg.Caption = fmt.Sprintf("📚 %s\n\n%s", tutorialTitle, tutorial.Bio)

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
		}
	}

	// Faylni o'qish
	fileData, err := ioutil.ReadFile(dataFile)
	if err != nil {
		log.Printf("Faylni o'qishda xatolik: %v", err)
		return BotData{
			Tutorials: make(map[string]Tutorial),
			Admins:    make(map[string]AdminInfo),
		}
	}

	// JSON formatdan dekodlash
	err = json.Unmarshal(fileData, &data)
	if err != nil {
		log.Printf("JSON dekodlashda xatolik: %v", err)
		return BotData{
			Tutorials: make(map[string]Tutorial),
			Admins:    make(map[string]AdminInfo),
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
