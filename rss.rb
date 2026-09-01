# rss.rb
# RSS Reader (Фильтрация новостей) на Ruby

require 'json'
require 'net/http'
require 'uri'
require 'rexml/document'
include REXML

# ANSI-цвета
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

def colorize(text, color)
  "#{color}#{text}#{RESET}"
end

class Filter
  attr_accessor :keywords, :days, :source, :unread_only, :read_only

  def initialize(keywords: [], days: 0, source: nil, unread_only: false, read_only: false)
    @keywords = keywords
    @days = days
    @source = source
    @unread_only = unread_only
    @read_only = read_only
  end
end

class RssReader
  def initialize(data_file = 'rss_data.json')
    @data_file = data_file
    load_data
  end

  def load_data
    if File.exist?(@data_file)
      @data = JSON.parse(File.read(@data_file), symbolize_names: true)
    else
      @data = { feeds: [], next_feed_id: 1, next_item_id: 1 }
    end
    @next_feed_id = @data[:next_feed_id]
    @next_item_id = @data[:next_item_id]
    f = @data[:filter] || {}
    @filter = Filter.new(
      keywords: f[:keywords] || [],
      days: f[:days] || 0,
      source: f[:source],
      unread_only: f[:unread_only] || false,
      read_only: f[:read_only] || false
    )
  end

  def save_data
    @data[:next_feed_id] = @next_feed_id
    @data[:next_item_id] = @next_item_id
    @data[:filter] = {
      keywords: @filter.keywords,
      days: @filter.days,
      source: @filter.source,
      unread_only: @filter.unread_only,
      read_only: @filter.read_only
    }
    File.write(@data_file, JSON.pretty_generate(@data))
  end

  def fetch_feed(url)
    uri = URI.parse(url)
    http = Net::HTTP.new(uri.host, uri.port)
    http.use_ssl = (uri.scheme == 'https')
    http.open_timeout = 10
    http.read_timeout = 10
    response = http.get(uri.request_uri)
    response.body
  rescue => e
    raise "Не удалось загрузить ленту: #{e.message}"
  end

  def parse_feed(content)
    doc = Document.new(content)
    root = doc.root
    title = 'Без названия'
    items = []

    if root.name == 'rss'
      channel = root.e
