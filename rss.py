# rss.py
# RSS Reader (Фильтрация новостей) на Python

import sys
import os
import json
import time
import datetime
import argparse
import xml.etree.ElementTree as ET
import urllib.request
import subprocess
import re
from typing import List, Dict, Optional, Any

# ANSI-цвета (тёмная тема)
RESET = "\033[0m"
BOLD = "\033[1m"
RED = "\033[91m"
GREEN = "\033[92m"
YELLOW = "\033[93m"
BLUE = "\033[94m"
MAGENTA = "\033[95m"
CYAN = "\033[96m"
WHITE = "\033[97m"
GRAY = "\033[90m"

def colorize(text: str, color: str) -> str:
    return f"{color}{text}{RESET}"

class Filter:
    def __init__(self, keywords=None, days=None, source=None, unread_only=False, read_only=False):
        self.keywords = keywords or []
        self.days = days
        self.source = source
        self.unread_only = unread_only
        self.read_only = read_only

    def to_dict(self):
        return {
            "keywords": self.keywords,
            "days": self.days,
            "source": self.source,
            "unread_only": self.unread_only,
            "read_only": self.read_only
        }

    @classmethod
    def from_dict(cls, data):
        return cls(
            keywords=data.get("keywords", []),
            days=data.get("days"),
            source=data.get("source"),
            unread_only=data.get("unread_only", False),
            read_only=data.get("read_only", False)
        )

class RssReader:
    def __init__(self, data_file="rss_data.json"):
        self.data_file = data_file
        self.data = self._load()
        self.next_feed_id = self.data.get("next_feed_id", 1)
        self.next_item_id = self.data.get("next_item_id", 1)
        self.filters = Filter.from_dict(self.data.get("filters", {}))

    def _load(self):
        if os.path.exists(self.data_file):
            with open(self.data_file, 'r', encoding='utf-8') as f:
                return json.load(f)
        return {"feeds": [], "next_feed_id": 1, "next_item_id": 1, "filters": {}}

    def _save(self):
        self.data["next_feed_id"] = self.next_feed_id
        self.data["next_item_id"] = self.next_item_id
        self.data["filters"] = self.filters.to_dict()
        with open(self.data_file, 'w', encoding='utf-8') as f:
            json.dump(self.data, f, indent=2, ensure_ascii=False)

    def _fetch_feed(self, url):
        try:
            with urllib.request.urlopen(url, timeout=10) as response:
                content = response.read().decode('utf-8')
                return content
        except Exception as e:
            raise Exception(f"Не удалось загрузить ленту: {e}")

    def _parse_feed(self, content):
        root = ET.fromstring(content)
        if root.tag == 'rss':
            return self._parse_rss(root)
        elif root.tag.endswith('feed'):
            return self._parse_atom(root)
        else:
            raise Exception("Неизвестный формат ленты")

    def _parse_rss(self, root):
        channel = root.find('channel')
        title = channel.find('title').text if channel.find('title') is not None else "Без названия"
        items = []
        for item in channel.findall('item'):
            title_elem = item.find('title')
            link_elem = item.find('link')
            pub_date_elem = item.find('pubDate')
            desc_elem = item.find('description')
            categories = [cat.text for cat in item.findall('category') if cat.text]
            items.append({
                'title': title_elem.text if title_elem is not None else "Без заголовка",
                'link': link_elem.text if link_elem is not None else "",
                'pubDate': pub_date_elem.text if pub_date_elem is not None else "",
                'description': desc_elem.text if desc_elem is not None else "",
                'categories': categories
            })
        return title, items

    def _parse_atom(self, root):
        title = root.find('title').text if root.find('title') is not None else "Без названия"
        items = []
        for entry in root.findall('entry'):
            title_elem = entry.find('title')
            link_elem = entry.find('link')
            pub_date_elem = entry.find('published')
            if pub_date_elem is None:
                pub_date_elem = entry.find('updated')
            desc_elem = entry.find('summary')
            if desc_elem is None:
                desc_elem = entry.find('content')
            categories = [cat.get('term') for cat in entry.findall('category') if cat.get('term')]
            items.append({
                'title': title_elem.text if title_elem is not None else "Без заголовка",
                'link': link_elem.get('href') if link_elem is not None else "",
                'pubDate': pub_date_elem.text if pub_date_elem is not None else "",
                'description': desc_elem.text if desc_elem is not None else "",
                'categories': categories
            })
        return title, items

    def add_feed(self, url, title=None):
        if not title:
            try:
                content = self._fetch_feed(url)
                title, _ = self._parse_feed(content)
            except:
                title = url
        feed = {
            "id": self.next_feed_id,
            "title": title,
            "url": url,
            "items": [],
            "last_fetch": None
        }
        self.data["feeds"].append(feed)
        self.next_feed_id += 1
        self._save()
        return feed

    def remove_feed(self, feed_id=None, url=None):
        for i, f in enumerate(self.data["feeds"]):
            if (feed_id is not None and f["id"] == feed_id) or (url is not None and f["url"] == url):
                del self.data["feeds"][i]
                self._save()
                return True
        return False

    def list_feeds(self):
        return self.data["feeds"]

    def fetch_all(self, filter_obj: Optional[Filter] = None):
        if filter_obj is None:
            filter_obj = self.filters
        for feed in self.data["feeds"]:
            try:
                content = self._fetch_feed(feed["url"])
                title, items = self._parse_feed(content)
                if title != feed["title"]:
                    feed["title"] = title
                new_items = []
                for item in items:
                    exists = any(i["link"] == item["link"] for i in feed["items"])
                    if not exists:
                        item["id"] = self.next_item_id
                        item["read"] = False
                        new_items.append(item)
                        self.next_item_id += 1
                feed["items"].extend(new_items)
                feed["last_fetch"] = datetime.datetime.now().isoformat()
            except Exception as e:
                print(colorize(f"Ошибка при загрузке {feed['url']}: {e}", RED))
        self._save()
        # Применяем фильтры
        all_items = []
        for feed in self.data["feeds"]:
            for item in feed["items"]:
                all_items.append((feed["title"], item))
        # Фильтрация
        all_items = self._apply_filters(all_items, filter_obj)
        # Сортировка по дате (новые сверху)
        all_items.sort(key=lambda x: x[1].get("pubDate", ""), reverse=True)
        return all_items

    def _apply_filters(self, items, filter_obj: Filter):
        result = []
        for feed_title, item in items:
            # Фильтр по ключевым словам
            if filter_obj.keywords:
                text = (item.get("title", "") + " " + item.get("description", "") + " " + " ".join(item.get("categories", []))).lower()
                if not any(kw.lower() in text for kw in filter_obj.keywords):
                    continue
            # Фильтр по дате
            if filter_obj.days:
                try:
                    pub = datetime.datetime.fromisoformat(item.get("pubDate", "").replace('Z', '+00:00'))
                    if (datetime.datetime.now(datetime.timezone.utc) - pub).days > filter_obj.days:
                        continue
                except:
                    pass
            # Фильтр по источнику
            if filter_obj.source and filter_obj.source.lower() not in feed_title.lower():
                continue
            # Фильтр по статусу
            if filter_obj.unread_only and item.get("read", False):
                continue
            if filter_obj.read_only and not item.get("read", False):
                continue
            result.append((feed_title, item))
        return result

    def get_item(self, item_id):
        for feed in self.data["feeds"]:
            for item in feed["items"]:
                if item["id"] == item_id:
                    return feed["title"], item
        return None, None

    def mark_read(self, item_id):
        for feed in self.data["feeds"]:
            for item in feed["items"]:
                if item["id"] == item_id:
                    item["read"] = True
                    self._save()
                    return True
        return False

    def open_link(self, url):
        if sys.platform == 'darwin':
            subprocess.run(['open', url])
        elif sys.platform == 'win32':
            subprocess.run(['start', url], shell=True)
        else:
            subprocess.run(['xdg-open', url])

    def export_opml(self, filename="subscriptions.opml"):
        opml = '<?xml version="1.0" encoding="UTF-8"?>\n'
        opml += '<opml version="1.0">\n<head>\n<title>RSS Subscriptions</title>\n</head>\n<body>\n'
        for feed in self.data["feeds"]:
            opml += f'<outline text="{feed["title"]}" title="{feed["title"]}" type="rss" xmlUrl="{feed["url"]}"/>\n'
        opml += '</body>\n</opml>'
        with open(filename, 'w', encoding='utf-8') as f:
            f.write(opml)
        return filename

    def import_opml(self, filename):
        try:
            tree = ET.parse(filename)
            root = tree.getroot()
            for outline in root.findall('.//outline'):
                url = outline.get('xmlUrl')
                title = outline.get('text') or outline.get('title') or url
                if url:
                    self.add_feed(url, title)
            self._save()
            return True
        except Exception as e:
            print(colorize(f"Ошибка импорта OPML: {e}", RED))
            return False

    def save_filters(self):
        self._save()
        print(colorize("Фильтры сохранены", GREEN))

    def load_filters(self):
        # уже загружены в __init__
        print(colorize("Фильтры загружены", GREEN))

    def clear_filters(self):
        self.filters = Filter()
        self._save()
        print(colorize("Фильтры очищены", GREEN))

def main():
    parser = argparse.ArgumentParser(description="RSS Reader (Фильтрация новостей)")
    parser.add_argument("command", choices=["add", "remove", "list", "fetch", "read", "filter", "save-filters", "load-filters", "export", "import", "help"],
                        help="Команда")
    parser.add_argument("--url", help="URL ленты")
    parser.add_argument("--title", help="Название ленты")
    parser.add_argument("--id", type=int, help="ID ленты или новости")
    parser.add_argument("--file", help="Имя файла для импорта/экспорта")
    parser.add_argument("--data", default="rss_data.json", help="Файл данных")
    parser.add_argument("--open", action="store_true", default=True, help="Открыть ссылку в браузере")
    parser.add_argument("--text", action="store_true", help="Показать текст в консоли")
    # Опции фильтрации
    parser.add_argument("--keyword", action="append", help="Ключевое слово для фильтрации (можно несколько)")
    parser.add_argument("--days", type=int, help="Показывать новости за последние N дней")
    parser.add_argument("--source", help="Фильтр по источнику (название ленты)")
    parser.add_argument("--unread", action="store_true", help="Показывать только непрочитанные")
    parser.add_argument("--read", action="store_true", help="Показывать только прочитанные")
    parser.add_argument("--clear-filters", action="store_true", help="Очистить все фильтры")
    args = parser.parse_args()

    reader = RssReader(args.data)

    if args.command == "help":
        print(__doc__)
        sys.exit(0)

    # Обработка фильтров
    if args.clear_filters:
        reader.clear_filters()
        return

    if args.command == "filter" or args.keyword or args.days or args.source or args.unread or args.read:
        # Обновляем фильтры
        if args.keyword:
            reader.filters.keywords = args.keyword
        if args.days:
            reader.filters.days = args.days
        if args.source:
            reader.filters.source = args.source
        if args.unread:
            reader.filters.unread_only = True
            reader.filters.read_only = False
        if args.read:
            reader.filters.read_only = True
            reader.filters.unread_only = False
        reader._save()
        print(colorize("Фильтры обновлены", GREEN))
        return

    if args.command == "save-filters":
        reader.save_filters()
        return

    if args.command == "load-filters":
        reader.load_filters()
        return

    if args.command == "add":
        if not args.url:
            print("Требуется --url")
            sys.exit(1)
        feed = reader.add_feed(args.url, args.title)
        print(colorize(f"Лента добавлена: {feed['title']} (ID {feed['id']})", GREEN))

    elif args.command == "remove":
        if args.id is None and not args.url:
            print("Требуется --id или --url")
            sys.exit(1)
        if reader.remove_feed(args.id, args.url):
            print(colorize("Лента удалена", GREEN))
        else:
            print(colorize("Лента не найдена", RED))

    elif args.command == "list":
        feeds = reader.list_feeds()
        if not feeds:
            print("Нет лент.")
        else:
            print(colorize("Список лент:", BOLD + CYAN))
            for f in feeds:
                print(f"  {colorize(str(f['id']), GREEN)}: {f['title']} ({f['url']})")

    elif args.command == "fetch":
        filter_obj = reader.filters  # используем текущие
        items = reader.fetch_all(filter_obj)
        if not items:
            print("Новостей нет (возможно, фильтры слишком строгие).")
        else:
            print(colorize("Новости (отфильтрованные):", BOLD + CYAN))
            for feed_title, item in items:
                status = "🔵" if not item.get("read", False) else "⚪"
                date = item.get("pubDate", "")[:16]
                print(f"{status} {colorize(str(item['id']), GREEN)} | {colorize(item['title'][:60], WHITE)} | {colorize(feed_title, GRAY)} | {colorize(date, YELLOW)}")

    elif args.command == "read":
        if args.id is None:
            print("Требуется --id")
            sys.exit(1)
        feed_title, item = reader.get_item(args.id)
        if not item:
            print(colorize("Новость не найдена", RED))
        else:
            reader.mark_read(args.id)
            print(colorize(f"Заголовок: {item['title']}", BOLD + WHITE))
            print(colorize(f"Дата: {item.get('pubDate', '')}", YELLOW))
            print(colorize(f"Источник: {feed_title}", GRAY))
            if args.text:
                print(colorize("Содержание:", BOLD))
                print(item.get("description", "Нет описания"))
            else:
                print(colorize(f"Ссылка: {item.get('link', '')}", CYAN))
                if item.get('link'):
                    try:
                        reader.open_link(item['link'])
                        print(colorize("Ссылка открыта в браузере", GREEN))
                    except:
                        print(colorize("Не удалось открыть браузер", RED))

    elif args.command == "export":
        filename = args.file or "subscriptions.opml"
        reader.export_opml(filename)
        print(colorize(f"Подписки экспортированы в {filename}", GREEN))

    elif args.command == "import":
        if not args.file:
            print("Требуется --file")
            sys.exit(1)
        if reader.import_opml(args.file):
            print(colorize("Подписки импортированы", GREEN))

if __name__ == "__main__":
    main()
