#!/usr/bin/env ruby

require 'optparse'

class Benchmark
  AB = 'ab'
  GO_AB = 'go-ab'
  HEY = 'hey'

  AB_PATTERN = /Requests per second:\s+([\d\.]+)\s+\[#\/sec\]/
  HEY_PATTERN = /Requests\/sec:\s+([\d\.]+)/

  def initialize(opts)
    @url = opts[:url]
    @requests = opts[:requests]
    @min_concurrency = 1
    @max_concurrency = opts[:max_concurrency]
    @step = opts[:step]
    @interval_sec = opts[:interval_sec]

    @concurrencies = (@min_concurrency..@max_concurrency).select { |n| n == 1 || (n % @step).zero? }
    @ab_results = []
    @go_ab_results = []
    @hey_results = []
  end

  def run
    @concurrencies.each do |concurrency|
      STDERR.print "concurrency: #{concurrency}\r"
      @ab_results << do_request(AB, '-q', concurrency, AB_PATTERN)
      @go_ab_results << do_request(GO_AB, '-q', concurrency, AB_PATTERN)
      @hey_results << do_request(HEY, '', concurrency, HEY_PATTERN)
      sleep(@interval_sec)
    end
    STDERR.puts ""
  end

  def output
    puts ['concurrency', *@concurrencies].join("\t")
    puts [AB, *@ab_results].join("\t")
    puts [GO_AB, *@go_ab_results].join("\t")
    puts [HEY, *@hey_results].join("\t")
  end

  private

  def do_request(cmd, opts, concurrency, pattern)
    throughput = nil
    `#{cmd} #{opts} -n #{@requests} -c #{concurrency} #{@url}`.match(pattern) { |m| throughput = m[1] }
    throughput.to_f
  end
end

STDOUT.sync = true

option_parser = OptionParser.new
opts = {
  url: 'http://127.0.0.1:8000/',
  requests: 1_000,
  max_concurrency: 100,
  step: 10,
  interval_sec: 30,
}

option_parser.on('-u', '--url URL') { |v| opts[:url] = v }
option_parser.on('-n', '--requests Number of Requests') { |v| opts[:requests] = v.to_i }
option_parser.on('-c', '--concurrency Max concurrency') { |v| opts[:max_concurrency] = v.to_i }
option_parser.on('-s', '--step Number of increment step') { |v| opts[:step] = v.to_i }
option_parser.on('-i', '--interval Interval(sec) between benchmark') { |v| opts[:interval_sec] = v.to_i }

option_parser.parse!(ARGV)

b = Benchmark.new(opts)
b.run
b.output
