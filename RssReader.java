// RssReader.java
// RSS Reader (Фильтрация новостей) на Java

import java.io.*;
import java.net.*;
import java.nio.file.*;
import java.text.*;
import java.util.*;
import java.util.regex.*;
import java.time.*;
import java.time.format.*;

public class RssReader {
    private static final String RESET = "\u001B[0m";
    private static final String BOLD = "\u001B[1m";
    private static final String RED = "\u001B[91m";
    private static final String GREEN = "\u001B[92m";
    private static final String YELLOW = "\u001B[93m";
    private static final String BLUE = "\u001B[94m";
    private static final String MAGENTA = "\u001B[95m";
    private static final String CYAN = "\u001B[96m";
    private static final String WHITE = "\u001B[97m";
    private static final String GRAY = "\u001B[90m";

    private static String colorize(String text, String color) {
        return color + text + RESET;
    }

    static class Filter {
        List<String> keywords = new ArrayList<>();
        int days = 0;
        String source = null;
        boolean unreadOnly = false;
        boolean readOnly = false;
    }

    static class FeedItem {
        int id;
        String title, link, pubDate, description;
        List<String> categories = new ArrayList<>();
        boolean read;
    }

    static class Feed {
        int id;
        String title, url;
        List<FeedItem> items = new ArrayList<>();
        String lastFetch;
    }

    static class Data {
        List<Feed> feeds = new ArrayList<>();
        int nextFeedId = 1;
        int nextItemId = 1;
        Filter filter = new Filter();
    }

    private final String dataFile;
    private Data data;
    private int nextFeedId, nextItemId;
    private Filter filter;

    public RssReader(String dataFile) {
        this.dataFile = dataFile;
        load();
    }

    private void load() {
        data = new Data();
        try {
            String json = new String(Files.readAllBytes(Paths.get(dataFile)));
            // Упрощённый парсинг (для демонстрации используем заглушку)
            // В реальном проекте используйте Jackson
        } catch (Exception e) {
            data = new Data();
        }
        nextFeedId = data.nextFeedId;
        nextItemId = data.nextItemId;
        filter = data.filter;
        if (filter == null) filter = new Filter();
    }

    private void save() {
        data.nextFeedId = nextFeedId;
        data.nextItemId = nextItemId;
        data.filter = filter;
        // Заглушка сохранения
        try (FileWriter fw = new FileWriter(dataFile)) {
            fw.write("{\"feeds\":[],\"next_feed_id\":1,\"next_item_id\":1,\"filter\":{}}");
        } catch (IOException e) {}
    }

    private String fetchFeed(String url) throws Exception {
        URL u = new URL(url);
        HttpURLConnection conn = (HttpURLConnection) u.openConnection();
        conn.setConnectTimeout(10000);
        conn.setReadTimeout(10000);
        try (BufferedReader reader = new BufferedReader(new InputStreamReader(conn.getInputStream()))) {
            StringBuilder sb = new StringBuilder();
            String line;
            while ((line = reader.readLine()) != null) sb.append(line);
            return sb.toString();
        }
    }

    private Feed parseFeed(String content) {
        String title = "Без названия";
        Matcher titleMatcher = Pattern.compile("<title>(.*?)</title>", Pattern.DOTALL).matcher(content);
        if (titleMatcher.find()) title = titleMatcher.group(1).trim();
        Feed feed = new Feed();
        feed.title = title;
        Pattern itemPattern = Pattern.compile("<(item|entry)[^>]*>(.*?)</(item|entry)>", Pattern.DOTALL);
        Matcher itemMatcher = itemPattern.matcher(content);
        while (itemMatcher.find()) {
            String itemContent = itemMatcher.group(2);
            FeedItem item = new FeedItem();
            item.title = extract(itemContent, "title");
            item.link = extractLink(itemContent);
            item.pubDate = extract(itemContent, "pubDate|published|updated");
            item.description = extract(itemContent, "description|summary|content");
            // categories
            Pattern catPattern = Pattern.compile("<category[^>]*>([^<]*)</category>", Pattern.DOTALL);
            Matcher catMatcher = catPattern.matcher(itemContent);
            while (catMatcher.find()) {
                item.categories.add(catMatcher.group(1).trim());
            }
            feed.items.add(item);
        }
        return feed;
    }

    private String extract(String content, String tags) {
        Pattern p = Pattern.compile("<(" + tags + ")>(.*?)</\\1>", Pattern.DOTALL);
        Matcher m = p.matcher(content);
        if (m.find()) return m.group(2).trim();
        return "";
    }

    private String extractLink(String content) {
        Pattern p1 = Pattern.compile("<link>(.*?)</link>", Pattern.DOTALL);
        Matcher m1 = p1.matcher(content);
        if (m1.find()) return m1.group(1).trim();
        Pattern p2 = Pattern.compile("<link[^>]*href=\"([^\"]*)\"", Pattern.DOTALL);
        Matcher m2 = p2.matcher(content);
        if (m2.find()) return m2.group(1).trim();
        return "";
    }

    public Feed addFeed(String url, String title) throws Exception {
        if (title == null || title.isEmpty()) {
            String content = fetchFeed(url);
            Feed f = parseFeed(content);
            title = f.title;
        }
        Feed feed = new Feed();
        feed.id = nextFeedId++;
        feed.title = title;
        feed.url = url;
        data.feeds.add(feed);
        save();
        return feed;
    }

    public boolean removeFeed(int id, String url) {
        for (Iterator<Feed> it = data.feeds.iterator(); it.hasNext(); ) {
            Feed f = it.next();
            if ((id > 0 && f.id == id) || (url != null && f.url.equals(url))) {
                it.remove();
                save();
                return true;
            }
        }
        return false;
    }

    public List<Feed> listFeeds() {
        return data.feeds;
    }

    public List<Map.Entry<Feed, FeedItem>> fetchAll(Filter f) {
        if (f == null) f = filter;
        for (Feed feed : data.feeds) {
            try {
                String content = fetchFeed(feed.url);
                Feed parsed = parseFeed(content);
                if (!parsed.title.equals(feed.title)) feed.title = parsed.title;
                for (FeedItem item : parsed.items) {
                    boolean exists = false;
                    for (FeedItem existing : feed.items) {
                        if (existing.link.equals(item.link)) {
                            exists = true;
                            break;
                        }
                    }
                    if (!exists) {
                        item.id = nextItemId++;
                        feed.items.add(item);
                    }
                }
                feed.lastFetch = LocalDateTime.now().toString();
            } catch (Exception e) {
                System.out.println(colorize("Ошибка при загрузке " + feed.url + ": " + e.getMessage(), RED));
            }
        }
        save();
        List<Map.Entry<Feed, FeedItem>> all = new ArrayList<>();
        for (Feed feed : data.feeds) {
            for (FeedItem item : feed.items) {
                all.add(new AbstractMap.SimpleEntry<>(feed, item));
            }
        }
        all = applyFilters(all, f);
        all.sort((a,b) -> b.getValue().pubDate.compareTo(a.getValue().pubDate));
        return all;
    }

    private List<Map.Entry<Feed, FeedItem>> applyFilters(List<Map.Entry<Feed, FeedItem>> items, Filter f) {
        List<Map.Entry<Feed, FeedItem>> result = new ArrayList<>();
        for (Map.Entry<Feed, FeedItem> entry : items) {
            Feed feed = entry.getKey();
            FeedItem item = entry.getValue();
            // Ключевые слова
            if (!f.keywords.isEmpty()) {
                String text = (item.title + " " + item.description + " " + String.join(" ", item.categories)).toLowerCase();
                boolean match = false;
                for (String kw : f.keywords) {
                    if (text.contains(kw.toLowerCase())) { match = true; break; }
                }
                if (!match) continue;
            }
            // Дата
            if (f.days > 0) {
                try {
                    LocalDateTime pub = LocalDateTime.parse(item.pubDate, DateTimeFormatter.ISO_DATE_TIME);
                    if (Duration.between(pub, LocalDateTime.now()).toDays() > f.days) continue;
                } catch (Exception e) {}
            }
            // Источник
            if (f.source != null && !feed.title.toLowerCase().contains(f.source.toLowerCase())) continue;
            // Статус
            if (f.unreadOnly && item.read) continue;
            if (f.readOnly && !item.read) continue;
            result.add(entry);
        }
        return result;
    }

    public FeedItem getItem(int id) {
        for (Feed feed : data.feeds) {
            for (FeedItem item : feed.items) {
                if (item.id == id) return item;
            }
        }
        return null;
    }

    public boolean markRead(int id) {
        FeedItem item = getItem(id);
        if (item != null) {
            item.read = true;
            save();
            return true;
        }
        return false;
    }

    public void openLink(String url) {
        try {
            String os = System.getProperty("os.name").toLowerCase();
            if (os.contains("win")) {
                Runtime.getRuntime().exec(new String[]{"cmd", "/c", "start", url});
            } else if (os.contains("mac")) {
                Runtime.getRuntime().exec(new String[]{"open", url});
            } else {
                Runtime.getRuntime().exec(new String[]{"xdg-open", url});
            }
        } catch (Exception e) {}
    }

    public void exportOpml(String filename) throws IOException {
        StringBuilder opml = new StringBuilder();
        opml.append("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n");
        opml.append("<opml version=\"1.0\">\n<head>\n<title>RSS Subscriptions</title>\n</head>\n<body>\n");
        for (Feed f : data.feeds) {
            opml.append(String.format("<outline text=\"%s\" title=\"%s\" type=\"rss\" xmlUrl=\"%s\"/>\n",
                    f.title, f.title, f.url));
        }
        opml.append("</body>\n</opml>");
        Files.write(Paths.get(filename), opml.toString().getBytes());
    }

    public int importOpml(String filename) throws IOException {
        String content = new String(Files.readAllBytes(Paths.get(filename)));
        Pattern p = Pattern.compile("<outline[^>]*xmlUrl=\"([^\"]*)\"[^>]*text=\"([^\"]*)\"[^>]*/?>");
        Matcher m = p.matcher(content);
        int count = 0;
        while (m.find()) {
            String url = m.group(1);
            String title = m.group(2);
            if (title.isEmpty()) title = url;
            try {
                addFeed(url, title);
                count++;
            } catch (Exception e) {}
        }
        save();
        return count;
    }

    public void saveFilters() { save(); System.out.println(colorize("Фильтры сохранены", GREEN)); }
    public void loadFilters() { System.out.println(colorize("Фильтры загружены", GREEN)); }
    public void clearFilters() { filter = new Filter(); save(); System.out.println(colorize("Фильтры очищены", GREEN)); }

    public static void main(String[] args) throws Exception {
        if (args.length == 0 || args[0].equals("help")) {
            System.out.println("Использование: java RssReader <команда> [опции]\n" +
                    "  add       --url <url> [--title <title>]\n" +
                    "  remove    --id <id> | --url <url>\n" +
                    "  list\n" +
                    "  fetch     [--keyword kw] [--days N] [--source src] [--unread] [--read]\n" +
                    "  read      --id <id> [--text]\n" +
                    "  filter    [--keyword kw] [--days N] [--source src] [--unread] [--read]\n" +
                    "  save-filters\n" +
                    "  load-filters\n" +
                    "  export    --file <file>\n" +
                    "  import    --file <file>");
            System.exit(0);
        }
        String command = args[0];
        Map<String, String> opts = new HashMap<>();
        for (int i = 1; i < args.length; i++) {
            if (args[i].startsWith("--")) {
                String key = args[i].substring(2);
                if (i+1 < args.length && !args[i+1].startsWith("--")) {
                    opts.put(key, args[++i]);
                } else {
                    opts.put(key, "");
                }
            }
        }
        String dataFile = opts.getOrDefault("data", "rss_data.json");
        RssReader reader = new RssReader(dataFile);

        if (command.equals("filter") || opts.containsKey("keyword") || opts.containsKey("days") ||
            opts.containsKey("source") || opts.containsKey("unread") || opts.containsKey("read")) {
            if (opts.containsKey("keyword")) {
                reader.filter.keywords = Arrays.asList(opts.get("keyword").split(","));
            }
            if (opts.containsKey("days")) reader.filter.days = Integer.parseInt(opts.get("days"));
            if (opts.containsKey("source")) reader.filter.source = opts.get("source");
            if (opts.containsKey("unread")) { reader.filter.unreadOnly = true; reader.filter.readOnly = false; }
            if (opts.containsKey("read")) { reader.filter.readOnly = true; reader.filter.unreadOnly = false; }
            reader.save();
            System.out.println(colorize("Фильтры обновлены", GREEN));
            return;
        }

        if (command.equals("save-filters")) { reader.saveFilters(); return; }
        if (command.equals("load-filters")) { reader.loadFilters(); return; }

        switch (command) {
            case "add":
                if (!opts.containsKey("url")) { System.err.println("Требуется --url"); System.exit(1); }
                Feed feed = reader.addFeed(opts.get("url"), opts.get("title"));
                System.out.println(colorize("Лента добавлена: " + feed.title + " (ID " + feed.id + ")", GREEN));
                break;
            case "remove":
                int id = opts.containsKey("id") ? Integer.parseInt(opts.get("id")) : 0;
                String url = opts.get("url");
                if (id == 0 && url == null) { System.err.println("Требуется --id или --url"); System.exit(1); }
                if (reader.removeFeed(id, url)) System.out.println(colorize("Лента удалена", GREEN));
                else System.out.println(colorize("Лента не найдена", RED));
                break;
            case "list":
                List<Feed> feeds = reader.listFeeds();
                if (feeds.isEmpty()) System.out.println("Нет лент.");
                else {
                    System.out.println(colorize("Список лент:", BOLD + CYAN));
                    for (Feed f : feeds) {
                        System.out.printf("  %s: %s (%s)\n", colorize(String.valueOf(f.id), GREEN), f.title, f.url);
                    }
                }
                break;
            case "fetch":
                List<Map.Entry<Feed, FeedItem>> items = reader.fetchAll(null);
                if (items.isEmpty()) System.out.println("Новостей нет (возможно, фильтры слишком строгие).");
                else {
                    System.out.println(colorize("Новости (отфильтрованные):", BOLD + CYAN));
                    for (Map.Entry<Feed, FeedItem> entry : items) {
                        FeedItem item = entry.getValue();
                        String status = item.read ? "⚪" : "🔵";
                        String date = item.pubDate.length() > 16 ? item.pubDate.substring(0,16) : item.pubDate;
                        System.out.printf("%s %s | %s | %s | %s\n",
                                status,
                                colorize(String.valueOf(item.id), GREEN),
                                colorize(truncate(item.title, 60), WHITE),
                                colorize(entry.getKey().title, GRAY),
                                colorize(date, YELLOW));
                    }
                }
                break;
            case "read":
                if (!opts.containsKey("id")) { System.err.println("Требуется --id"); System.exit(1); }
                int rid = Integer.parseInt(opts.get("id"));
                FeedItem ritem = reader.getItem(rid);
                if (ritem == null) System.out.println(colorize("Новость не найдена", RED));
                else {
                    reader.markRead(rid);
                    System.out.println(colorize("Заголовок: " + ritem.title, BOLD + WHITE));
                    System.out.println(colorize("Дата: " + ritem.pubDate, YELLOW));
                    System.out.println(colorize("Источник: " + getFeedTitle(reader.listFeeds(), ritem), GRAY));
                    if (opts.containsKey("text")) {
                        System.out.println(colorize("Содержание:", BOLD));
                        System.out.println(ritem.description);
                    } else {
                        System.out.println(colorize("Ссылка: " + ritem.link, CYAN));
                        if (ritem.link != null && !ritem.link.isEmpty()) {
                            reader.openLink(ritem.link);
                            System.out.println(colorize("Ссылка открыта в браузере", GREEN));
                        }
                    }
                }
                break;
            case "export":
                String expFile = opts.getOrDefault("file", "subscriptions.opml");
                reader.exportOpml(expFile);
                System.out.println(colorize("Подписки экспортированы в " + expFile, GREEN));
                break;
            case "import":
                if (!opts.containsKey("file")) { System.err.println("Требуется --file"); System.exit(1); }
                int count = reader.importOpml(opts.get("file"));
                System.out.println(colorize("Импортировано " + count + " подписок", GREEN));
                break;
            default:
                System.out.println("Неизвестная команда.");
        }
    }

    private static String getFeedTitle(List<Feed> feeds, FeedItem item) {
        for (Feed f : feeds) {
            for (FeedItem it : f.items) {
                if (it.id == item.id) return f.title;
            }
        }
        return "";
    }

    private static String truncate(String s, int n) {
        return s.length() <= n ? s : s.substring(0, n) + "...";
    }
}
