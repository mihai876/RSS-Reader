// rss.ts
// RSS Reader (Фильтрация новостей) на TypeScript

import * as fs from 'fs';
import * as https from 'https';
import * as http from 'http';
import { URL } from 'url';
import { exec } from 'child_process';

const RESET = '\x1b[0m';
const BOLD = '\x1b[1m';
const RED = '\x1b[91m';
const GREEN = '\x1b[92m';
const YELLOW = '\x1b[93m';
const BLUE = '\x1b[94m';
const MAGENTA = '\x1b[95m';
const CYAN = '\x1b[96m';
const WHITE = '\x1b[97m';
const GRAY = '\x1b[90m';

function colorize(text: string, color: string): string {
    return `${color}${text}${RESET}`;
}

interface FilterData {
    keywords?: string[];
    days?: number;
    source?: string | null;
    unread_only?: boolean;
    read_only?: boolean;
}

class Filter {
    keywords: string[];
    days: number | null;
    source: string | null;
    unreadOnly: boolean;
    readOnly: boolean;

    constructor(keywords: string[] = [], days: number | null = null, source: string | null = null,
                unreadOnly: boolean = false, readOnly: boolean = false) {
        this.keywords = keywords;
        this.days = days;
        this.source = source;
        this.unreadOnly = unreadOnly;
        this.readOnly = readOnly;
    }
}

interface FeedItem {
    id: number;
    title: string;
    link: string;
    pubDate: string;
    description: string;
    categories: string[];
    read: boolean;
}

interface Feed {
    id: number;
    title: string;
    url: string;
    items: FeedItem[];
    last_fetch: string | null;
}

interface Data {
    feeds: Feed[];
    next_feed_id: number;
    next_item_id: number;
    filters?: FilterData;
}

class RssReader {
    private data: Data;
    private nextFeedId: number;
    private nextItemId: number;
    public filters: Filter;

    constructor(private dataFile: string = 'rss_data.json') {
        this.data = this.load();
        this.nextFeedId = this.data.next_feed_id || 1;
        this.nextItemId = this.data.next_item_id || 1;
        const f = this.data.filters || {};
        this.filters = new Filter(
            f.keywords || [],
            f.days || null,
            f.source || null,
            f.unread_only || false,
            f.read_only || false
        );
    }

    private load(): Data {
        try {
            if (fs.existsSync(this.dataFile)) {
                return JSON.parse(fs.readFileSync(this.dataFile, 'utf-8'));
            }
        } catch (e) {}
        return { feeds: [], next_feed_id: 1, next_item_id: 1, filters: {} };
    }

    private save(): void {
        this.data.next_feed_id = this.nextFeedId;
        this.data.next_item_id = this.nextItemId;
        this.data.filters = {
            keywords: this.filters.keywords,
            days: this.filters.days,
            source: this.filters.source,
            unread_only: this.filters.unreadOnly,
            read_only: this.filters.readOnly
        };
        fs.writeFileSync(this.dataFile, JSON.stringify(this.data, null, 2), 'utf-8');
    }

    private fetchFeed(url: string): Promise<string> {
        return new Promise((resolve, reject) => {
            const parsed = new URL(url);
            const client = parsed.protocol === 'https:' ? https : http;
            client.get(url, (res) => {
                let data = '';
                res.on('data', chunk => data += chunk);
                res.on('end', () => resolve(data));
            }).on('error', reject);
        });
    }

    private parseFeed(content: string): { title: string; items: Omit<FeedItem, 'id' | 'read'>[] } {
        const items: Omit<FeedItem, 'id' | 'read'>[] = [];
        let title = 'Без названия';
        const titleMatch = content.match(/<title>(.*?)<\/title>/i);
        if (titleMatch) title = titleMatch[1];
        const itemRegex = /<(item|entry)[^>]*>([\s\S]*?)<\/(item|entry)>/gi;
        let match;
        while ((match = itemRegex.exec(content)) !== null) {
            const itemContent = match[2];
            const item: any = {};
            const titleEl = itemContent.match(/<title>(.*?)<\/title>/i);
            const linkEl = itemContent.match(/<link[^>]*>(.*?)<\/link>/i) || itemContent.match(/<link[^>]*href="([^"]*)"/i);
            const pubEl = itemContent.match(/<(pubDate|published|updated)>(.*?)<\/(pubDate|published|updated)>/i);
            const descEl = itemContent.match(/<(description|summary|content)>(.*?)<\/(description|summary|content)>/i);
            const catEls = itemContent.match(/<category[^>]*>([^<]*)<\/category>/gi);
            item.title = titleEl ? titleEl[1].trim() : 'Без заголовка';
            item.link = linkEl ? (linkEl[1] || linkEl[0]) : '';
            item.pubDate = pubEl ? pubEl[2].trim() : '';
            item.description = descEl ? descEl[2].trim() : '';
            item.categories = catEls ? catEls.map(c => c.replace(/<[^>]*>/g,'').trim()) : [];
            items.push(item);
        }
        return { title, items };
    }

    addFeed(url: string, title?: string): Promise<Feed> {
        if (title) {
            return Promise.resolve(this._addFeed(url, title));
        } else {
            return this.fetchFeed(url).then(content => {
                const parsed = this.parseFeed(content);
                return this._addFeed(url, parsed.title || url);
            }).catch(() => {
                return this._addFeed(url, url);
            });
        }
    }

    private _addFeed(url: string, title: string): Feed {
        const feed: Feed = {
            id: this.nextFeedId++,
            title,
            url,
            items: [],
            last_fetch: null
        };
        this.data.feeds.push(feed);
        this.save();
        return feed;
    }

    removeFeed(feedId?: number, url?: string): boolean {
        const idx = this.data.feeds.findIndex(f => (feedId && f.id === feedId) || (url && f.url === url));
        if (idx !== -1) {
            this.data.feeds.splice(idx, 1);
            this.save();
            return true;
        }
        return false;
    }

    listFeeds(): Feed[] {
        return this.data.feeds;
    }

    fetchAll(filterObj?: Filter): Promise<{ feed: string; item: FeedItem }[]> {
        if (!filterObj) filterObj = this.filters;
        const promises = this.data.feeds.map(feed => {
            return this.fetchFeed(feed.url).then(content => {
                const parsed = this.parseFeed(content);
                if (parsed.title !== feed.title) feed.title = parsed.title;
                const newItems: FeedItem[] = [];
                for (const item of parsed.items) {
                    const exists = feed.items.some(i => i.link === item.link);
                    if (!exists) {
                        const newItem: FeedItem = {
                            ...item,
                            id: this.nextItemId++,
                            read: false
                        };
                        newItems.push(newItem);
                    }
                }
                feed.items.push(...newItems);
                feed.last_fetch = new Date().toISOString();
            }).catch(err => {
                console.log(colorize(`Ошибка при загрузке ${feed.url}: ${err.message}`, RED));
            });
        });
        return Promise.all(promises).then(() => {
            this.save();
            const all: { feed: string; item: FeedItem }[] = [];
            for (const feed of this.data.feeds) {
                for (const item of feed.items) {
                    all.push({ feed: feed.title, item });
                }
            }
            const filtered = this._applyFilters(all, filterObj);
            filtered.sort((a,b) => (a.item.pubDate < b.item.pubDate ? 1 : -1));
            return filtered;
        });
    }

    private _applyFilters(items: { feed: string; item: FeedItem }[], filterObj: Filter): { feed: string; item: FeedItem }[] {
        return items.filter(({ feed, item }) => {
            // Ключевые слова
            if (filterObj.keywords && filterObj.keywords.length > 0) {
                const text = (item.title + ' ' + item.description + ' ' + (item.categories || []).join(' ')).toLowerCase();
                if (!filterObj.keywords.some(kw => text.includes(kw.toLowerCase()))) return false;
            }
            // Дата
            if (filterObj.days) {
                try {
                    const pub = new Date(item.pubDate);
                    const now = new Date();
                    const diffDays = (now.getTime() - pub.getTime()) / (1000 * 60 * 60 * 24);
                    if (diffDays > filterObj.days) return false;
                } catch (e) {}
            }
            // Источник
            if (filterObj.source && !feed.toLowerCase().includes(filterObj.source.toLowerCase())) return false;
            // Статус
            if (filterObj.unreadOnly && item.read) return false;
            if (filterObj.readOnly && !item.read) return false;
            return true;
        });
    }

    getItem(itemId: number): { feedTitle: string; item: FeedItem } | null {
        for (const feed of this.data.feeds) {
            for (const item of feed.items) {
                if (item.id === itemId) {
                    return { feedTitle: feed.title, item };
                }
            }
        }
        return null;
    }

    markRead(itemId: number): boolean {
        const res = this.getItem(itemId);
        if (res) {
            res.item.read = true;
            this.save();
            return true;
        }
        return false;
    }

    openLink(url: string): void {
        const cmd = process.platform === 'darwin' ? 'open' :
                    process.platform === 'win32' ? 'start' : 'xdg-open';
        exec(`${cmd} "${url}"`);
    }

    exportOpml(filename: string = 'subscriptions.opml'): string {
        let opml = '<?xml version="1.0" encoding="UTF-8"?>\n';
        opml += '<opml version="1.0">\n<head>\n<title>RSS Subscriptions</title>\n</head>\n<body>\n';
        for (const feed of this.data.feeds) {
            opml += `<outline text="${feed.title}" title="${feed.title}" type="rss" xmlUrl="${feed.url}"/>\n`;
        }
        opml += '</body>\n</opml>';
        fs.writeFileSync(filename, opml, 'utf-8');
        return filename;
    }

    importOpml(filename: string): number {
        const content = fs.readFileSync(filename, 'utf-8');
        const regex = /<outline[^>]*xmlUrl="([^"]*)"[^>]*text="([^"]*)"[^>]*\/?>/gi;
        let match;
        let count = 0;
        while ((match = regex.exec(content)) !== null) {
            const url = match[1];
            const title = match[2] || url;
            this._addFeed(url, title);
            count++;
        }
        this.save();
        return count;
    }

    saveFilters(): void {
        this.save();
        console.log(colorize('Фильтры сохранены', GREEN));
    }

    loadFilters(): void {
        console.log(colorize('Фильтры загружены', GREEN));
    }

    clearFilters(): void {
        this.filters = new Filter();
        this.save();
        console.log(colorize('Фильтры очищены', GREEN));
    }
}

function main(): void {
    const args = process.argv.slice(2);
    if (args.length === 0 || args[0] === 'help') {
        console.log(`Использование: ts-node rss.ts <команда> [опции]
  add       --url <url> [--title <title>]
  remove    --id <id> | --url <url>
  list
  fetch     [--keyword <kw>] [--days N] [--source <src>] [--unread] [--read]
  read      --id <id> [--text]
  filter    [--keyword <kw>] [--days N] [--source <src>] [--unread] [--read]
  save-filters
  load-filters
  export    [--file <file>]
  import    --file <file>
  help`);
        process.exit(0);
    }

    const command = args[0];
    const options: any = {};
    for (let i = 1; i < args.length; i++) {
        const arg = args[i];
        if (arg.startsWith('--')) {
            const key = arg.slice(2);
            const value = args[++i];
            options[key] = value;
        }
    }

    const dataFile = options.data || 'rss_data.json';
    const reader = new RssReader(dataFile);

    // Обработка фильтров
    if (command === 'filter' || options.keyword || options.days || options.source || options.unread || options.read) {
        if (options.keyword) {
            const kw = Array.isArray(options.keyword) ? options.keyword : [options.keyword];
            reader.filters.keywords = kw;
        }
        if (options.days) reader.filters.days = parseInt(options.days);
        if (options.source) reader.filters.source = options.source;
        if (options.unread) { reader.filters.unreadOnly = true; reader.filters.readOnly = false; }
        if (options.read) { reader.filters.readOnly = true; reader.filters.unreadOnly = false; }
        reader.save();
        console.log(colorize('Фильтры обновлены', GREEN));
        return;
    }

    if (command === 'save-filters') {
        reader.saveFilters();
        return;
    }
    if (command === 'load-filters') {
        reader.loadFilters();
        return;
    }

    switch (command) {
        case 'add': {
            if (!options.url) { console.error('Требуется --url'); process.exit(1); }
            reader.addFeed(options.url, options.title).then(feed => {
                console.log(colorize(`Лента добавлена: ${feed.title} (ID ${feed.id})`, GREEN));
            }).catch(err => console.error(colorize(`Ошибка: ${err.message}`, RED)));
            break;
        }
        case 'remove': {
            if (!options.id && !options.url) { console.error('Требуется --id или --url'); process.exit(1); }
            const id = options.id ? parseInt(options.id) : null;
            if (reader.removeFeed(id, options.url)) {
                console.log(colorize('Лента удалена', GREEN));
            } else {
                console.log(colorize('Лента не найдена', RED));
            }
            break;
        }
        case 'list': {
            const feeds = reader.listFeeds();
            if (!feeds.length) console.log('Нет лент.');
            else {
                console.log(colorize('Список лент:', BOLD + CYAN));
                for (const f of feeds) {
                    console.log(`  ${colorize(f.id, GREEN)}: ${f.title} (${f.url})`);
                }
            }
            break;
        }
        case 'fetch': {
            reader.fetchAll(reader.filters).then(items => {
                if (!items.length) {
                    console.log('Новостей нет (возможно, фильтры слишком строгие).');
                } else {
                    console.log(colorize('Новости (отфильтрованные):', BOLD + CYAN));
                    for (const { feed, item } of items) {
                        const status = item.read ? '⚪' : '🔵';
                        const date = (item.pubDate || '').slice(0,16);
                        console.log(`${status} ${colorize(item.id, GREEN)} | ${colorize(item.title.slice(0,60), WHITE)} | ${colorize(feed, GRAY)} | ${colorize(date, YELLOW)}`);
                    }
                }
            });
            break;
        }
        case 'read': {
            if (!options.id) { console.error('Требуется --id'); process.exit(1); }
            const id = parseInt(options.id);
            const res = reader.getItem(id);
            if (!res) {
                console.log(colorize('Новость не найдена', RED));
            } else {
                reader.markRead(id);
                console.log(colorize(`Заголовок: ${res.item.title}`, BOLD + WHITE));
                console.log(colorize(`Дата: ${res.item.pubDate}`, YELLOW));
                console.log(colorize(`Источник: ${res.feedTitle}`, GRAY));
                if (options.text) {
                    console.log(colorize('Содержание:', BOLD));
                    console.log(res.item.description || 'Нет описания');
                } else {
                    console.log(colorize(`Ссылка: ${res.item.link}`, CYAN));
                    if (res.item.link) {
                        try {
                            reader.openLink(res.item.link);
                            console.log(colorize('Ссылка открыта в браузере', GREEN));
                        } catch (e) {
                            console.log(colorize('Не удалось открыть браузер', RED));
                        }
                    }
                }
            }
            break;
        }
        case 'export': {
            const filename = options.file || 'subscriptions.opml';
            reader.exportOpml(filename);
            console.log(colorize(`Подписки экспортированы в ${filename}`, GREEN));
            break;
        }
        case 'import': {
            if (!options.file) { console.error('Требуется --file'); process.exit(1); }
            const count = reader.importOpml(options.file);
            console.log(colorize(`Импортировано ${count} подписок`, GREEN));
            break;
        }
        default:
            console.log('Неизвестная команда.');
    }
}

main();
