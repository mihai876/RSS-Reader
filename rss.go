// rss.go
// RSS Reader (Фильтрация новостей) на Go

package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ANSI-цвета
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[91m"
	green  = "\033[92m"
	yellow = "\033[93m"
	blue   = "\033[94m"
	magenta= "\033[95m"
	cyan   = "\033[96m"
	white  = "\033[97m"
	gray   = "\033[90m"
)

func colorize(text, color string) string {
	return color + text + reset
}

// Фильтр
type Filter struct {
	Keywords   []string `json:"keywords"`
	Days       int      `json:"days"`
	Source     string   `json:"source"`
	UnreadOnly bool     `json:"unread_only"`
	ReadOnly   bool     `json:"read_only"`
}

// Структуры RSS
type RSS struct {
	Channel struct {
		Title string `xml:"title"`
		Items []struct {
			Title       string   `xml:"title"`
			Link        string   `xml:"link"`
			PubDate     string   `xml:"pubDate"`
			Description string   `xml:"description"`
			Categories  []string `xml:"category"`
		} `xml:"item"`
	} `xml:"channel"`
}

type Atom struct {
	Title string `xml:"title"`
	Entries []struct {
		Title     string   `xml:"title"`
		Link      struct { Href string `xml:"href,attr"` } `xml:"link"`
		Published string   `xml:"published"`
		Updated   string   `xml:"updated"`
		Summary   string   `xml:"summary"`
		Content   string   `xml:"content"`
		Categories []struct { Term string `xml:"term,attr"` } `xml:"category"`
	} `xml:"entry"`
}

type FeedItem struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Link        string   `json:"link"`
	PubDate     string   `json:"pubDate"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	Read        bool     `json:"read"`
}

type Feed struct {
	ID        int        `json:"id"`
	Title     string     `json:"title"`
	URL       string     `json:"url"`
	Items     []FeedItem `json:"items"`
	LastFetch string     `json:"last_fetch"`
}

type Data struct {
	Feeds      []Feed `json:"feeds"`
	NextFeedID int    `json:"next_feed_id"`
	NextItemID int    `json:"next_item_id"`
	Filter     Filter `json:"filter"`
}

type RssReader struct {
	dataFile string
	data     Data
	nextFeedID int
	nextItemID int
	filter   Filter
}

func NewRssReader(dataFile string) *RssReader {
	r := &RssReader{dataFile: dataFile}
	r.load()
	return r
}

func (r *RssReader) load() {
	content, err := ioutil.ReadFile(r.dataFile)
	if err != nil {
		r.data = Data{Feeds: []Feed{}, NextFeedID: 1, NextItemID: 1, Filter: Filter{}}
	} else {
		json.Unmarshal(content, &r.data)
	}
	r.nextFeedID = r.data.NextFeedID
	r.nextItemID = r.data.NextItemID
	r.filter = r.data.Filter
}

func (r *RssReader) save() {
	r.data.NextFeedID = r.nextFeedID
	r.data.NextItemID = r.nextItemID
	r.data.Filter = r.filter
	data, _ := json.MarshalIndent(r.data, "", "  ")
	ioutil.WriteFile(r.dataFile, data, 0644)
}

func (r *RssReader) fetchFeed(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	return string(body), err
}

func (r *RssReader) parseFeed(content string) (string, []FeedItem) {
	// Попробуем RSS
	var rss RSS
	if err := xml.Unmarshal([]byte(content), &rss); err == nil && rss.Channel.Title != "" {
		items := []FeedItem{}
		for _, it := range rss.Channel.Items {
			items = append(items, FeedItem{
				Title:       it.Title,
				Link:        it.Link,
				PubDate:     it.PubDate,
				Description: it.Description,
				Categories:  it.Categories,
			})
		}
		return rss.Channel.Title, items
	}
	// Попробуем Atom
	var atom Atom
	if err := xml.Unmarshal([]byte(content), &atom); err == nil && atom.Title != "" {
		items := []FeedItem{}
		for _, it := range atom.Entries {
			pubDate := it.Published
			if pubDate == "" {
				pubDate = it.Updated
			}
			cats := []string{}
			for _, c := range it.Categories {
				cats = append(cats, c.Term)
			}
			items = append(items, FeedItem{
				Title:       it.Title,
				Link:        it.Link.Href,
				PubDate:     pubDate,
				Description: it.Summary,
				Categories:  cats,
			})
		}
		return atom.Title, items
	}
	return "Без названия", []FeedItem{}
}

func (r *RssReader) addFeed(url, title string) Feed {
	if title == "" {
		content, err := r.fetchFeed(url)
		if err == nil {
			t, _ := r.parseFeed(content)
			title = t
		}
		if title == "" {
			title = url
		}
	}
	feed := Feed{
		ID:        r.nextFeedID,
		Title:     title,
		URL:       url,
		Items:     []FeedItem{},
		LastFetch: "",
	}
	r.nextFeedID++
	r.data.Feeds = append(r.data.Feeds, feed)
	r.save()
	return feed
}

func (r *RssReader) removeFeed(id int, url string) bool {
	for i, f := range r.data.Feeds {
		if (id > 0 && f.ID == id) || (url != "" && f.URL == url) {
			r.data.Feeds = append(r.data.Feeds[:i], r.data.Feeds[i+1:]...)
			r.save()
			return true
		}
	}
	return false
}

func (r *RssReader) listFeeds() []Feed {
	return r.data.Feeds
}

func (r *RssReader) fetchAll(filter Filter) []struct{ Feed string; Item FeedItem } {
	for i := range r.data.Feeds {
		feed := &r.data.Feeds[i]
		content, err := r.fetchFeed(feed.URL)
		if err != nil {
			fmt.Println(colorize("Ошибка при загрузке "+feed.URL+": "+err.Error(), red))
			continue
		}
		title, items := r.parseFeed(content)
		if title != feed.Title {
			feed.Title = title
		}
		for _, it := range items {
			exists := false
			for _, existing := range feed.Items {
				if existing.Link == it.Link {
					exists = true
					break
				}
			}
			if !exists {
				it.ID = r.nextItemID
				it.Read = false
				feed.Items = append(feed.Items, it)
				r.nextItemID++
			}
		}
		feed.LastFetch = time.Now().Format(time.RFC3339)
	}
	r.save()

	var all []struct{ Feed string; Item FeedItem }
	for _, feed := range r.data.Feeds {
		for _, item := range feed.Items {
			all = append(all, struct{ Feed string; Item FeedItem }{feed.Title, item})
		}
	}
	// Фильтрация
	filtered := r.applyFilters(all, filter)
	// Сортировка по дате
	for i := 0; i < len(filtered); i++ {
		for j := i + 1; j < len(filtered); j++ {
			if filtered[i].Item.PubDate < filtered[j].Item.PubDate {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}
	return filtered
}

func (r *RssReader) applyFilters(items []struct{ Feed string; Item FeedItem }, filter Filter) []struct{ Feed string; Item FeedItem } {
	var result []struct{ Feed string; Item FeedItem }
	for _, it := range items {
		// Ключевые слова
		if len(filter.Keywords) > 0 {
			text := strings.ToLower(it.Item.Title + " " + it.Item.Description + " " + strings.Join(it.Item.Categories, " "))
			match := false
			for _, kw := range filter.Keywords {
				if strings.Contains(text, strings.ToLower(kw)) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		// Дата
		if filter.Days > 0 {
			pub, err := time.Parse(time.RFC3339, it.Item.PubDate)
			if err == nil {
				diff := time.Since(pub).Hours() / 24
				if diff > float64(filter.Days) {
					continue
				}
			}
		}
		// Источник
		if filter.Source != "" && !strings.Contains(strings.ToLower(it.Feed), strings.ToLower(filter.Source)) {
			continue
		}
		// Статус
		if filter.UnreadOnly && it.Item.Read {
			continue
		}
		if filter.ReadOnly && !it.Item.Read {
			continue
		}
		result = append(result, it)
	}
	return result
}

func (r *RssReader) getItem(id int) (string, *FeedItem) {
	for _, feed := range r.data.Feeds {
		for i := range feed.Items {
			if feed.Items[i].ID == id {
				return feed.Title, &feed.Items[i]
			}
		}
	}
	return "", nil
}

func (r *RssReader) markRead(id int) bool {
	_, item := r.getItem(id)
	if item != nil {
		item.Read = true
		r.save()
		return true
	}
	return false
}

func openLink(url string) {
	var cmd string
	switch os.Getenv("OS") {
	case "Windows_NT":
		cmd = "start"
	default:
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = "xdg-open"
		} else {
			cmd = "open"
		}
	}
	exec.Command(cmd, url).Run()
}

func (r *RssReader) exportOpml(filename string) {
	opml := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="1.0">
<head><title>RSS Subscriptions</title></head>
<body>
`
	for _, feed := range r.data.Feeds {
		opml += fmt.Sprintf(`<outline text="%s" title="%s" type="rss" xmlUrl="%s"/>`+"\n", feed.Title, feed.Title, feed.URL)
	}
	opml += `</body>
</opml>`
	ioutil.WriteFile(filename, []byte(opml), 0644)
}

func (r *RssReader) importOpml(filename string) int {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return 0
	}
	re := regexp.MustCompile(`<outline[^>]*xmlUrl="([^"]*)"[^>]*text="([^"]*)"[^>]*/?>`)
	matches := re.FindAllStringSubmatch(string(content), -1)
	count := 0
	for _, match := range matches {
		url := match[1]
		title := match[2]
		if title == "" {
			title = url
		}
		r.addFeed(url, title)
		count++
	}
	r.save()
	return count
}

func (r *RssReader) saveFilters() {
	r.save()
	fmt.Println(colorize("Фильтры сохранены", green))
}

func (r *RssReader) loadFilters() {
	// уже загружены
	fmt.Println(colorize("Фильтры загружены", green))
}

func (r *RssReader) clearFilters() {
	r.filter = Filter{}
	r.save()
	fmt.Println(colorize("Фильтры очищены", green))
}

func main() {
	var (
		cmd        string
		archive    string
		output     string
		keyword    string
		days       int
		source     string
		unread     bool
		read       bool
		clear      bool
		file       string
	)
	flag.StringVar(&cmd, "cmd", "", "Команда: add, remove, list, fetch, read, filter, save-filters, load-filters, export, import")
	flag.StringVar(&archive, "a", "", "Архив (не используется в RSS)")
	flag.StringVar(&output, "o", "", "Выходной файл")
	flag.StringVar(&keyword, "keyword", "", "Ключевое слово (можно несколько через запятую)")
	flag.IntVar(&days, "days", 0, "Показывать за последние N дней")
	flag.StringVar(&source, "source", "", "Фильтр по источнику")
	flag.BoolVar(&unread, "unread", false, "Только непрочитанные")
	flag.BoolVar(&read, "read", false, "Только прочитанные")
	flag.BoolVar(&clear, "clear-filters", false, "Очистить фильтры")
	flag.StringVar(&file, "file", "", "Файл для импорта/экспорта OPML")
	flag.Usage = func() {
		fmt.Println(`Использование: go run rss.go -cmd <команда> [опции]
  add       -url <url> -title <title>
  remove    -id <id> | -url <url>
  list
  fetch     [-keyword kw] [-days N] [-source src] [-unread] [-read]
  read      -id <id> [-text]
  filter    [-keyword kw] [-days N] [-source src] [-unread] [-read]
  save-filters
  load-filters
  export    -file <file>
  import    -file <file>`)
	}
	flag.Parse()

	if cmd == "" {
		fmt.Println("Укажите -cmd")
		os.Exit(1)
	}

	dataFile := "rss_data.json"
	reader := NewRssReader(dataFile)

	// Обработка фильтров
	if cmd == "filter" || keyword != "" || days > 0 || source != "" || unread || read || clear {
		if clear {
			reader.clearFilters()
			return
		}
		if keyword != "" {
			reader.filter.Keywords = strings.Split(keyword, ",")
		}
		if days > 0 {
			reader.filter.Days = days
		}
		if source != "" {
			reader.filter.Source = source
		}
		if unread {
			reader.filter.UnreadOnly = true
			reader.filter.ReadOnly = false
		}
		if read {
			reader.filter.ReadOnly = true
			reader.filter.UnreadOnly = false
		}
		reader.save()
		fmt.Println(colorize("Фильтры обновлены", green))
		return
	}

	if cmd == "save-filters" {
		reader.saveFilters()
		return
	}
	if cmd == "load-filters" {
		reader.loadFilters()
		return
	}

	switch cmd {
	case "add":
		url := flag.Arg(0)
		title := flag.Arg(1)
		if url == "" {
			fmt.Println("Требуется URL")
			os.Exit(1)
		}
		feed := reader.addFeed(url, title)
		fmt.Printf(colorize("Лента добавлена: %s (ID %d)\n", green), feed.Title, feed.ID)
	case "remove":
		id := 0
		fmt.Sscanf(flag.Arg(0), "%d", &id)
		url := flag.Arg(0)
		if id == 0 && url == "" {
			fmt.Println("Требуется --id или --url")
			os.Exit(1)
		}
		if reader.removeFeed(id, url) {
			fmt.Println(colorize("Лента удалена", green))
		} else {
			fmt.Println(colorize("Лента не найдена", red))
		}
	case "list":
		feeds := reader.listFeeds()
		if len(feeds) == 0 {
			fmt.Println("Нет лент.")
		} else {
			fmt.Println(colorize("Список лент:", bold+cyan))
			for _, f := range feeds {
				fmt.Printf("  %s: %s (%s)\n", colorize(strconv.Itoa(f.ID), green), f.Title, f.URL)
			}
		}
	case "fetch":
		items := reader.fetchAll(reader.filter)
		if len(items) == 0 {
			fmt.Println("Новостей нет (возможно, фильтры слишком строгие).")
		} else {
			fmt.Println(colorize("Новости (отфильтрованные):", bold+cyan))
			for _, it := range items {
				status := "⚪"
				if !it.Item.Read {
					status = "🔵"
				}
				date := it.Item.PubDate
				if len(date) > 16 {
					date = date[:16]
				}
				fmt.Printf("%s %s | %s | %s | %s\n",
					status,
					colorize(strconv.Itoa(it.Item.ID), green),
					colorize(truncate(it.Item.Title, 60), white),
					colorize(it.Feed, gray),
					colorize(date, yellow))
			}
		}
	case "read":
		idStr := flag.Arg(0)
		if idStr == "" {
			fmt.Println("Требуется --id")
			os.Exit(1)
		}
		id, _ := strconv.Atoi(idStr)
		feedTitle, item := reader.getItem(id)
		if item == nil {
			fmt.Println(colorize("Новость не найдена", red))
		} else {
			reader.markRead(id)
			fmt.Println(colorize("Заголовок: "+item.Title, bold+white))
			fmt.Println(colorize("Дата: "+item.PubDate, yellow))
			fmt.Println(colorize("Источник: "+feedTitle, gray))
			if flag.Lookup("text") != nil && flag.Lookup("text").Value.String() == "true" {
				fmt.Println(colorize("Содержание:", bold))
				fmt.Println(item.Description)
			} else {
				fmt.Println(colorize("Ссылка: "+item.Link, cyan))
				if item.Link != "" {
					openLink(item.Link)
					fmt.Println(colorize("Ссылка открыта в браузере", green))
				}
			}
		}
	case "export":
		filename := file
		if filename == "" {
			filename = "subscriptions.opml"
		}
		reader.exportOpml(filename)
		fmt.Println(colorize("Подписки экспортированы в "+filename, green))
	case "import":
		if file == "" {
			fmt.Println("Требуется --file")
			os.Exit(1)
		}
		count := reader.importOpml(file)
		fmt.Println(colorize(fmt.Sprintf("Импортировано %d подписок", count), green))
	default:
		fmt.Println("Неизвестная команда.")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
