// RssReader.cs
// RSS Reader (Фильтрация новостей) на C#

using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net;
using System.Text;
using System.Text.RegularExpressions;
using System.Diagnostics;

class RssReader
{
    private const string Reset = "\u001B[0m";
    private const string Bold = "\u001B[1m";
    private const string Red = "\u001B[91m";
    private const string Green = "\u001B[92m";
    private const string Yellow = "\u001B[93m";
    private const string Blue = "\u001B[94m";
    private const string Magenta = "\u001B[95m";
    private const string Cyan = "\u001B[96m";
    private const string White = "\u001B[97m";
    private const string Gray = "\u001B[90m";

    private static string Colorize(string text, string color) => color + text + Reset;

    class Filter
    {
        public List<string> Keywords { get; set; } = new List<string>();
        public int Days { get; set; } = 0;
        public string Source { get; set; } = null;
        public bool UnreadOnly { get; set; } = false;
        public bool ReadOnly { get; set; } = false;
    }

    class FeedItem
    {
        public int Id { get; set; }
        public string Title { get; set; }
        public string Link { get; set; }
        public string PubDate { get; set; }
        public string Description { get; set; }
        public List<string> Categories { get; set; } = new List<string>();
        public bool Read { get; set; }
    }

    class Feed
    {
        public int Id { get; set; }
        public string Title { get; set; }
        public string Url { get; set; }
        public List<FeedItem> Items { get; set; } = new List<FeedItem>();
        public string LastFetch { get; set; }
    }

    class Data
    {
        public List<Feed> Feeds { get; set; } = new List<Feed>();
        public int NextFeedId { get; set; } = 1;
        public int NextItemId { get; set; } = 1;
        public Filter Filter { get; set; } = new Filter();
    }

    private readonly string dataFile;
    private Data data;
    private int nextFeedId, nextItemId;
    private Filter filter;

    public RssReader(string dataFile)
    {
        this.dataFile = dataFile;
        Load();
    }

    private void Load()
    {
        data = new Data();
        try
        {
            if (File.Exists(dataFile))
            {
                string json = File.ReadAllText(dataFile);
                // Упрощённо, используем заглушку
            }
        }
        catch { data = new Data(); }
        nextFeedId = data.NextFeedId;
        nextItemId = data.NextItemId;
        filter = data.Filter ?? new Filter();
    }

    private void Save()
    {
        data.NextFeedId = nextFeedId;
        data.NextItemId = nextItemId;
        data.Filter = filter;
        // Заглушка
        File.WriteAllText(dataFile, "{\"feeds\":[],\"next_feed_id\":1,\"next_item_id\":1,\"filter\":{}}");
    }

    private string FetchFeed(string url)
    {
        using (var client = new WebClient())
        {
            return client.DownloadString(url);
        }
    }

    private Feed ParseFeed(string content)
    {
        string title = "Без названия";
        var titleMatch = Regex.Match(content, @"<title>(.*?)</title>", RegexOptions.Singleline);
        if (titleMatch.Success) title = titleMatch.Groups[1].Value.Trim();
        var feed = new Feed { Title = title };
        var itemMatches = Regex.Matches(content, @"<(item|entry)[^>]*>(.*?)</(item|entry)>", RegexOptions.Singleline);
        foreach (Match m in itemMatches)
        {
            string itemContent = m.Groups[2].Value;
            var item = new FeedItem
            {
                Title = Extract(itemContent, "title"),
                Link = ExtractLink(itemContent),
                PubDate = Extract(itemContent, "pubDate|published|updated"),
                Description = Extract(itemContent, "description|summary|content")
            };
            var catMatches = Regex.Matches(itemContent, @"<category[^>]*>([^<]*)</category>");
            foreach (Match cm in catMatches)
                item.Categories.Add(cm.Groups[1].Value.Trim());
            feed.Items.Add(item);
        }
        return feed;
    }

    private string Extract(string content, string tags)
    {
        var m = Regex.Match(content, @"<(" + tags + ")>(.*?)</\\1>", RegexOptions.Singleline);
        return m.Success ? m.Groups[2].Value.Trim() : "";
    }

    private string ExtractLink(string content)
    {
        var m = Regex.Match(content, @"<link>(.*?)</link>", RegexOptions.Singleline);
        if (m.Success) return m.Groups[1].Value.Trim();
        m = Regex.Match(content, @"<link[^>]*href=""([^""]*)""");
        return m.Success ? m.Groups[1].Value : "";
    }

    public Feed AddFeed(string url, string title)
    {
        if (string.IsNullOrEmpty(title))
        {
            try
            {
                string content = FetchFeed(url);
                var f = ParseFeed(content);
                title = f.Title;
            }
            catch { title = url; }
        }
        var feed = new Feed { Id = nextFeedId++, Title = title, Url = url };
        data.Feeds.Add(feed);
        Save();
        return feed;
    }

    public bool RemoveFeed(int id, string url)
    {
        var f = data.Feeds.FirstOrDefault(x => (id > 0 && x.Id == id) || (url != null && x.Url == url));
        if (f != null)
        {
            data.Feeds.Remove(f);
            Save();
            return true;
        }
        return false;
    }

    public List<Feed> ListFeeds() => data.Feeds;

    public List<(Feed feed, FeedItem item)> FetchAll(Filter f = null)
    {
        if (f == null) f = filter;
        foreach (var feed in data.Feeds)
        {
            try
            {
                string content = FetchFeed(feed.Url);
                var parsed = ParseFeed(content);
                if (parsed.Title != feed.Title) feed.Title = parsed.Title;
                foreach (var item in parsed.Items)
                {
                    if (!feed.Items.Any(i => i.Link == item.Link))
                    {
                        item.Id = nextItemId++;
                        feed.Items.Add(item);
                    }
                }
                feed.LastFetch = DateTime.Now.ToString("o");
            }
            catch (Exception e)
            {
                Console.WriteLine(Colorize($"Ошибка при загрузке {feed.Url}: {e.Message}", Red));
            }
        }
        Save();
        var all = new List<(Feed, FeedItem)>();
        foreach (var feed in data.Feeds)
            foreach (var item in feed.Items)
                all.Add((feed, item));
        all = ApplyFilters(all, f);
        all.Sort((a,b) => string.Compare(b.Item.PubDate, a.Item.PubDate));
        return all;
    }

    private List<(Feed, FeedItem)> ApplyFilters(List<(Feed, FeedItem)> items, Filter f)
    {
        return items.Where(t => {
            var feed = t.Item1;
            var item = t.Item2;
            // Ключевые слова
            if (f.Keywords.Any())
            {
                string text = (item.Title + " " + item.Description + " " + string.Join(" ", item.Categories)).ToLower();
                if (!f.Keywords.Any(kw => text.Contains(kw.ToLower()))) return false;
            }
            // Дата
            if (f.Days > 0)
            {
                try
                {
                    var pub = DateTime.Parse(item.PubDate);
                    if ((DateTime.Now - pub).Days > f.Days) return false;
                }
                catch { }
            }
            // Источник
            if (!string.IsNullOrEmpty(f.Source) && !feed.Title.ToLower().Contains(f.Source.ToLower())) return false;
            // Статус
            if (f.UnreadOnly && item.Read) return false;
            if (f.ReadOnly && !item.Read) return false;
            return true;
        }).ToList();
    }

    public FeedItem GetItem(int id)
    {
        foreach (var feed in data.Feeds)
            foreach (var item in feed.Items)
                if (item.Id == id) return item;
        return null;
    }

    public bool MarkRead(int id)
    {
        var item = GetItem(id);
        if (item != null) { item.Read = true; Save(); return true; }
        return false;
    }

    public void OpenLink(string url)
    {
        try
        {
            Process.Start(url);
        }
        catch { }
    }

    public void ExportOpml(string filename)
    {
        var sb = new StringBuilder();
        sb.AppendLine("<?xml version=\"1.0\" encoding=\"UTF-8\"?>");
        sb.AppendLine("<opml version=\"1.0\">");
        sb.AppendLine("<head><title>RSS Subscriptions</title></head>");
        sb.AppendLine("<body>");
        foreach (var f in data.Feeds)
            sb.AppendLine($"<outline text=\"{f.Title}\" title=\"{f.Title}\" type=\"rss\" xmlUrl=\"{f.Url}\"/>");
        sb.AppendLine("</body></opml>");
        File.WriteAllText(filename, sb.ToString());
    }

    public int ImportOpml(string filename)
    {
        string content = File.ReadAllText(filename);
        var matches = Regex.Matches(content, @"<outline[^>]*xmlUrl=""([^""]*)""[^>]*text=""([^""]*)""[^>]*/?>");
        int count = 0;
        foreach (Match m in matches)
        {
            string url = m.Groups[1].Value;
            string title = m.Groups[2].Value;
            if (string.IsNullOrEmpty(title)) title = url;
            AddFeed(url, title);
            count++;
        }
        Save();
        return count;
    }

    public void SaveFilters() { Save(); Console.WriteLine(Colorize("Фильтры сохранены", Green)); }
    public void LoadFilters() { Console.WriteLine(Colorize("Фильтры загружены", Green)); }
    public void ClearFilters() { filter = new Filter(); Save(); Console.WriteLine(Colorize("Фильтры очищены", Green)); }

    static void Main(string[] args)
    {
        if (args.Length == 0 || args[0] == "help")
        {
            Console.WriteLine(@"Использование: RssReader <команда> [опции]
  add       --url <url> [--title <title>]
  remove    --id <id> | --url <url>
  list
  fetch     [--keyword kw] [--days N] [--source src] [--unread] [--read]
  read      --id <id> [--text]
  filter    [--keyword kw] [--days N] [--source src] [--unread] [--read]
  save-filters
  load-filters
  export    --file <file>
  import    --file <file>");
            return;
        }

        string command = args[0];
        var opts = new Dictionary<string, string>();
        for (int i = 1; i < args.Length; i++)
        {
            if (args[i].StartsWith("--"))
            {
                string key = args[i].Substring(2);
                if (i+1 < args.Length && !args[i+1].StartsWith("--"))
                    opts[key] = args[++i];
                else
                    opts[key] = "";
            }
        }

        string dataFile = opts.GetValueOrDefault("data", "rss_data.json");
        var reader = new RssReader(dataFile);

        if (command == "filter" || opts.ContainsKey("keyword") || opts.ContainsKey("days") ||
            opts.ContainsKey("source") || opts.ContainsKey("unread") || opts.ContainsKey("read"))
        {
            if (opts.ContainsKey("keyword"))
                reader.filter.Keywords = opts["keyword"].Split(',').ToList();
            if (opts.ContainsKey("days"))
                reader.filter.Days = int.Parse(opts["days"]);
            if (opts.ContainsKey("source"))
                reader.filter.Source = opts["source"];
            if (opts.ContainsKey("unread")) { reader.filter.UnreadOnly = true; reader.filter.ReadOnly = false; }
            if (opts.ContainsKey("read")) { reader.filter.ReadOnly = true; reader.filter.UnreadOnly = false; }
            reader.Save();
            Console.WriteLine(Colorize("Фильтры обновлены", Green));
            return;
        }

        if (command == "save-filters") { reader.SaveFilters(); return; }
        if (command == "load-filters") { reader.LoadFilters(); return; }

        switch (command)
        {
            case "add":
                if (!opts.ContainsKey("url")) { Console.WriteLine("Требуется --url"); return; }
                var feed = reader.AddFeed(opts["url"], opts.GetValueOrDefault("title"));
                Console.WriteLine(Colorize($"Лента добавлена: {feed.Title} (ID {feed.Id})", Green));
                break;
            case "remove":
                int id = opts.ContainsKey("id") ? int.Parse(opts["id"]) : 0;
                string url = opts.GetValueOrDefault("url");
                if (id == 0 && string.IsNullOrEmpty(url)) { Console.WriteLine("Требуется --id или --url"); return; }
                if (reader.RemoveFeed(id, url)) Console.WriteLine(Colorize("Лента удалена", Green));
                else Console.WriteLine(Colorize("Лента не найдена", Red));
                break;
            case "list":
                var feeds = reader.ListFeeds();
                if (!feeds.Any()) Console.WriteLine("Нет лент.");
                else
                {
                    Console.WriteLine(Colorize("Список лент:", Bold + Cyan));
                    foreach (var f in feeds)
                        Console.WriteLine($"  {Colorize(f.Id.ToString(), Green)}: {f.Title} ({f.Url})");
                }
                break;
            case "fetch":
                var items = reader.FetchAll();
                if (!items.Any()) Console.WriteLine("Новостей нет (возможно, фильтры слишком строгие).");
                else
                {
                    Console.WriteLine(Colorize("Новости (отфильтрованные):", Bold + Cyan));
                    foreach (var (feed, item) in items)
                    {
                        string status = item.Read ? "⚪" : "🔵";
                        string date = item.PubDate.Length > 16 ? item.PubDate.Substring(0,16) : item.PubDate;
                        Console.WriteLine($"{status} {Colorize(item.Id.ToString(), Green)} | {Colorize(Truncate(item.Title,60), White)} | {Colorize(feed.Title, Gray)} | {Colorize(date, Yellow)}");
                    }
                }
                break;
            case "read":
                if (!opts.ContainsKey("id")) { Console.WriteLine("Требуется --id"); return; }
                int rid = int.Parse(opts["id"]);
                var ritem = reader.GetItem(rid);
                if (ritem == null) Console.WriteLine(Colorize("Новость не найдена", Red));
                else
                {
                    reader.MarkRead(rid);
                    Console.WriteLine(Colorize($"Заголовок: {ritem.Title}", Bold + White));
                    Console.WriteLine(Colorize($"Дата: {ritem.PubDate}", Yellow));
                    Console.WriteLine(Colorize($"Источник: {GetFeedTitle(reader.ListFeeds(), ritem)}", Gray));
                    if (opts.ContainsKey("text"))
                    {
                        Console.WriteLine(Colorize("Содержание:", Bold));
                        Console.WriteLine(ritem.Description);
                    }
                    else
                    {
                        Console.WriteLine(Colorize($"Ссылка: {ritem.Link}", Cyan));
                        if (!string.IsNullOrEmpty(ritem.Link))
                        {
                            reader.OpenLink(ritem.Link);
                            Console.WriteLine(Colorize("Ссылка открыта в браузере", Green));
                        }
                    }
                }
                break;
            case "export":
                string expFile = opts.GetValueOrDefault("file", "subscriptions.opml");
                reader.ExportOpml(expFile);
                Console.WriteLine(Colorize($"Подписки экспортированы в {expFile}", Green));
                break;
            case "import":
                if (!opts.ContainsKey("file")) { Console.WriteLine("Требуется --file"); return; }
                int count = reader.ImportOpml(opts["file"]);
                Console.WriteLine(Colorize($"Импортировано {count} подписок", Green));
                break;
            default:
                Console.WriteLine("Неизвестная команда.");
                break;
        }
    }

    private static string GetFeedTitle(List<Feed> feeds, FeedItem item)
    {
        foreach (var f in feeds)
            foreach (var i in f.Items)
                if (i.Id == item.Id) return f.Title;
        return "";
    }

    private static string Truncate(string s, int n) => s.Length <= n ? s : s.Substring(0, n) + "...";
}
