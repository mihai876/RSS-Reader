<?php
// rss.php
// RSS Reader (Фильтрация новостей) на PHP

if (php_sapi_name() !== 'cli') {
    die("Это консольное приложение.\n");
}

// ANSI-цвета
define('RESET', "\033[0m");
define('BOLD', "\033[1m");
define('RED', "\033[91m");
define('GREEN', "\033[92m");
define('YELLOW', "\033[93m");
define('BLUE', "\033[94m");
define('MAGENTA', "\033[95m");
define('CYAN', "\033[96m");
define('WHITE', "\033[97m");
define('GRAY', "\033[90m");

function colorize($text, $color) {
    return $color . $text . RESET;
}

class Filter {
    public $keywords = [];
    public $days = 0;
    public $source = null;
    public $unreadOnly = false;
    public $readOnly = false;
}

class RssReader {
    private $dataFile;
    private $data;
    private $nextFeedId;
    private $nextItemId;
    public $filter;

    public function __construct($dataFile = 'rss_data.json') {
        $this->dataFile = $dataFile;
        $this->load();
    }

    private function load() {
        if (file_exists($this->dataFile)) {
            $json = file_get_contents($this->dataFile);
            $this->data = json_decode($json, true);
        }
        if (!$this->data) {
            $this->data = ['feeds' => [], 'next_feed_id' => 1, 'next_item_id' => 1];
        }
        $this->nextFeedId = $this->data['next_feed_id'] ?? 1;
        $this->nextItemId = $this->data['next_item_id'] ?? 1;
        $this->filter = new Filter();
        if (isset($this->data['filter'])) {
            $f = $this->data['filter'];
            $this->filter->keywords = $f['keywords'] ?? [];
            $this->filter->days = $f['days'] ?? 0;
            $this->filter->source = $f['source'] ?? null;
            $this->filter->unreadOnly = $f['unread_only'] ?? false;
            $this->filter->readOnly = $f['read_only'] ?? false;
        }
    }

    private function save() {
        $this->data['next_feed_id'] = $this->nextFeedId;
        $this->data['next_item_id'] = $this->nextItemId;
        $this->data['filter'] = [
            'keywords' => $this->filter->keywords,
            'days' => $this->filter->days,
            'source' => $this->filter->source,
            'unread_only' => $this->filter->unreadOnly,
            'read_only' => $this->filter->readOnly
        ];
        file_put_contents($this->dataFile, json_encode($this->data, JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE));
    }

    private function fetchFeed($url) {
        $ctx = stream_context_create(['http' => ['timeout' => 10]]);
        return file_get_contents($url, false, $ctx);
    }

    private function parseFeed($content) {
        $title = 'Без названия';
        if (preg_match('/<title>(.*?)<\/title>/is', $content, $m)) $title = trim($m[1]);
        $items = [];
        preg_match_all('/<(item|entry)[^>]*>(.*?)<\/(item|entry)>/is', $content, $matches, PREG_SET_ORDER);
        foreach ($matches as $match) {
            $itemContent = $match[2];
            $item = [
                'title' => $this->extract($itemContent, 'title'),
                'link' => $this->extractLink($itemContent),
                'pubDate' => $this->extract($itemContent, 'pubDate|published|updated'),
                'description' => $this->extract($itemContent, 'description|summary|content'),
                'categories' => []
            ];
            preg_match_all('/<category[^>]*>([^<]*)<\/category>/i', $itemContent, $cats);
            if ($cats[1]) $item['categories'] = array_map('trim', $cats[1]);
            $items[] = $item;
        }
        return ['title' => $title, 'items' => $items];
    }

    private function extract($content, $tags) {
        if (preg_match('/<(' . $tags . ')>(.*?)<\/\\1>/is', $content, $m)) {
            return trim($m[2]);
        }
        return '';
    }

    private function extractLink($content) {
        if (preg_match('/<link>(.*?)<\/link>/is', $content, $m)) return trim($m[1]);
        if (preg_match('/<link[^>]*href="([^"]*)"/is', $content, $m)) return $m[1];
        return '';
    }

    public function addFeed($url, $title = null) {
        if (!$title) {
            try {
                $content = $this->fetchFeed($url);
                $parsed = $this->parseFeed($content);
                $title = $parsed['title'];
            } catch (Exception $e) {
                $title = $url;
            }
        }
        $feed = [
            'id' => $this->nextFeedId,
            'title' => $title,
            'url' => $url,
            'items' => [],
            'last_fetch' => null
        ];
        $this->data['feeds'][] = $feed;
        $this->nextFeedId++;
        $this->save();
        return $feed;
    }

    public function removeFeed($id, $url) {
        foreach ($this->data['feeds'] as $i => $f) {
            if (($id && $f['id'] == $id) || ($url && $f['url'] == $url)) {
                array_splice($this->data['feeds'], $i, 1);
                $this->save();
                return true;
            }
        }
        return false;
    }

    public function listFeeds() {
        return $this->data['feeds'];
    }

    public function fetchAll($filter = null) {
        if (!$filter) $filter = $this->filter;
        foreach ($this->data['feeds'] as &$feed) {
            try {
                $content = $this->fetchFeed($feed['url']);
                $parsed = $this->parseFeed($content);
                if ($parsed['title'] != $feed['title']) $feed['title'] = $parsed['title'];
                foreach ($parsed['items'] as $item) {
                    $exists = false;
                    foreach ($feed['items'] as $existing) {
                        if ($existing['link'] == $item['link']) { $exists = true; break; }
                    }
                    if (!$exists) {
                        $item['id'] = $this->nextItemId;
                        $item['read'] = false;
                        $feed['items'][] = $item;
                        $this->nextItemId++;
                    }
                }
                $feed['last_fetch'] = date('c');
            } catch (Exception $e) {
                echo colorize("Ошибка при загрузке {$feed['url']}: " . $e->getMessage(), RED) . "\n";
            }
        }
        $this->save();
        $all = [];
        foreach ($this->data['feeds'] as $f) {
            foreach ($f['items'] as $item) {
                $all[] = ['feed' => $f['title'], 'item' => $item];
            }
        }
        $all = $this->applyFilters($all, $filter);
        usort($all, function($a, $b) {
            return strcmp($b['item']['pubDate'], $a['item']['pubDate']);
        });
        return $all;
    }

    private function applyFilters($items, $filter) {
        $result = [];
        foreach ($items as $it) {
            $feedTitle = $it['feed'];
            $item = $it['item'];
            // Ключевые слова
            if (!empty($filter->keywords)) {
                $text = strtolower($item['title'] . ' ' . $item['description'] . ' ' . implode(' ', $item['categories']));
                $match = false;
                foreach ($filter->keywords as $kw) {
                    if (strpos($text, strtolower($kw)) !== false) { $match = true; break; }
                }
                if (!$match) continue;
            }
            // Дата
            if ($filter->days > 0) {
                try {
                    $pub = new DateTime($item['pubDate']);
                    $diff = (new DateTime())->diff($pub)->days;
                    if ($diff > $filter->days) continue;
                } catch (Exception $e) {}
            }
            // Источник
            if ($filter->source && stripos($feedTitle, $filter->source) === false) continue;
            // Статус
            if ($filter->unreadOnly && $item['read']) continue;
            if ($filter->readOnly && !$item['read']) continue;
            $result[] = $it;
        }
        return $result;
    }

    public function getItem($id) {
        foreach ($this->data['feeds'] as $f) {
            foreach ($f['items'] as $item) {
                if ($item['id'] == $id) {
                    return ['feedTitle' => $f['title'], 'item' => $item];
                }
            }
        }
        return null;
    }

    public function markRead($id) {
        $res = $this->getItem($id);
        if ($res) {
            foreach ($this->data['feeds'] as &$f) {
                foreach ($f['items'] as &$item) {
                    if ($item['id'] == $id) {
                        $item['read'] = true;
                        $this->save();
                        return true;
                    }
                }
            }
        }
        return false;
    }

    public function openLink($url) {
        if (PHP_OS_FAMILY === 'Windows') exec("start $url");
        elseif (PHP_OS_FAMILY === 'Darwin') exec("open $url");
        else exec("xdg-open $url");
    }

    public function exportOpml($filename = 'subscriptions.opml') {
        $opml = '<?xml version="1.0" encoding="UTF-8"?>' . "\n";
        $opml .= '<opml version="1.0">' . "\n";
        $opml .= '<head><title>RSS Subscriptions</title></head>' . "\n";
        $opml .= '<body>' . "\n";
        foreach ($this->data['feeds'] as $f) {
            $opml .= '<outline text="' . htmlspecialchars($f['title']) . '" title="' . htmlspecialchars($f['title']) . '" type="rss" xmlUrl="' . htmlspecialchars($f['url']) . '"/>' . "\n";
        }
        $opml .= '</body></opml>';
        file_put_contents($filename, $opml);
        return $filename;
    }

    public function importOpml($filename) {
        $content = file_get_contents($filename);
        preg_match_all('/<outline[^>]*xmlUrl="([^"]*)"[^>]*text="([^"]*)"[^>]*\/?>/i', $content, $matches, PREG_SET_ORDER);
        $count = 0;
        foreach ($matches as $m) {
            $url = $m[1];
            $title = $m[2] ?: $url;
            $this->addFeed($url, $title);
            $count++;
        }
        $this->save();
        return $count;
    }

    public function saveFilters() { $this->save(); echo colorize("Фильтры сохранены", GREEN) . "\n"; }
    public function loadFilters() { echo colorize("Фильтры загружены", GREEN) . "\n"; }
    public function clearFilters() { $this->filter = new Filter(); $this->save(); echo colorize("Фильтры очищены", GREEN) . "\n"; }
}

$args = array_slice($argv, 1);
if (empty($args) || $args[0] == 'help') {
    echo "Использование: php rss.php <команда> [опции]\n";
    echo "  add       --url <url> [--title <title>]\n";
    echo "  remove    --id <id> | --url <url>\n";
    echo "  list\n";
    echo "  fetch     [--keyword kw] [--days N] [--source src] [--unread] [--read]\n";
    echo "  read      --id <id> [--text]\n";
    echo "  filter    [--keyword kw] [--days N] [--source src] [--unread] [--read]\n";
    echo "  save-filters\n";
    echo "  load-filters\n";
    echo "  export    --file <file>\n";
    echo "  import    --file <file>\n";
    exit(0);
}
$command = $args[0];
$options = [];
for ($i = 1; $i < count($args); $i++) {
    if (strpos($args[$i], '--') === 0) {
        $key = substr($args[$i], 2);
        if (isset($args[$i+1]) && strpos($args[$i+1], '--') !== 0) {
            $options[$key] = $args[++$i];
        } else {
            $options[$key] = '';
        }
    }
}

$dataFile = $options['data'] ?? 'rss_data.json';
$reader = new RssReader($dataFile);

if ($command == 'filter' || isset($options['keyword']) || isset($options['days']) ||
    isset($options['source']) || isset($options['unread']) || isset($options['read'])) {
    if (isset($options['keyword'])) {
        $reader->filter->keywords = explode(',', $options['keyword']);
    }
    if (isset($options['days'])) $reader->filter->days = (int)$options['days'];
    if (isset($options['source'])) $reader->filter->source = $options['source'];
    if (isset($options['unread'])) { $reader->filter->unreadOnly = true; $reader->filter->readOnly = false; }
    if (isset($options['read'])) { $reader->filter->readOnly = true; $reader->filter->unreadOnly = false; }
    $reader->save();
    echo colorize("Фильтры обновлены", GREEN) . "\n";
    exit(0);
}

if ($command == 'save-filters') { $reader->saveFilters(); exit(0); }
if ($command == 'load-filters') { $reader->loadFilters(); exit(0); }

switch ($command) {
    case 'add':
        if (empty($options['url'])) { echo "Требуется --url\n"; exit(1); }
        $feed = $reader->addFeed($options['url'], $options['title'] ?? null);
        echo colorize("Лента добавлена: {$feed['title']} (ID {$feed['id']})", GREEN) . "\n";
        break;
    case 'remove':
        $id = isset($options['id']) ? (int)$options['id'] : 0;
        $url = $options['url'] ?? null;
        if ($id == 0 && !$url) { echo "Требуется --id или --url\n"; exit(1); }
        if ($reader->removeFeed($id, $url)) echo colorize("Лента удалена", GREEN) . "\n";
        else echo colorize("Лента не найдена", RED) . "\n";
        break;
    case 'list':
        $feeds = $reader->listFeeds();
        if (empty($feeds)) echo "Нет лент.\n";
        else {
            echo colorize("Список лент:", BOLD . CYAN) . "\n";
            foreach ($feeds as $f) {
                echo "  " . colorize($f['id'], GREEN) . ": {$f['title']} ({$f['url']})\n";
            }
        }
        break;
    case 'fetch':
        $items = $reader->fetchAll();
        if (empty($items)) echo "Новостей нет (возможно, фильтры слишком строгие).\n";
        else {
            echo colorize("Новости (отфильтрованные):", BOLD . CYAN) . "\n";
            foreach ($items as $it) {
                $status = $it['item']['read'] ? '⚪' : '🔵';
                $date = substr($it['item']['pubDate'] ?? '', 0, 16);
                echo "$status " . colorize($it['item']['id'], GREEN) . " | " . colorize(substr($it['item']['title'], 0, 60), WHITE) . " | " . colorize($it['feed'], GRAY) . " | " . colorize($date, YELLOW) . "\n";
            }
        }
        break;
    case 'read':
        if (empty($options['id'])) { echo "Требуется --id\n"; exit(1); }
        $id = (int)$options['id'];
        $res = $reader->getItem($id);
        if (!$res) echo colorize("Новость не найдена", RED) . "\n";
        else {
            $reader->markRead($id);
            echo colorize("Заголовок: " . $res['item']['title'], BOLD . WHITE) . "\n";
            echo colorize("Дата: " . $res['item']['pubDate'], YELLOW) . "\n";
            echo colorize("Источник: " . $res['feedTitle'], GRAY) . "\n";
            if (isset($options['text'])) {
                echo colorize("Содержание:", BOLD) . "\n";
                echo $res['item']['description'] . "\n";
            } else {
                echo colorize("Ссылка: " . $res['item']['link'], CYAN) . "\n";
                if ($res['item']['link']) {
                    $reader->openLink($res['item']['link']);
                    echo colorize("Ссылка открыта в браузере", GREEN) . "\n";
                }
            }
        }
        break;
    case 'export':
        $filename = $options['file'] ?? 'subscriptions.opml';
        $reader->exportOpml($filename);
        echo colorize("Подписки экспортированы в $filename", GREEN) . "\n";
        break;
    case 'import':
        if (empty($options['file'])) { echo "Требуется --file\n"; exit(1); }
        $count = $reader->importOpml($options['file']);
        echo colorize("Импортировано $count подписок", GREEN) . "\n";
        break;
    default:
        echo "Неизвестная команда.\n";
}
