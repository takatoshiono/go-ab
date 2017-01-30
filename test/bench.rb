#!/usr/bin/env ruby

require 'optparse'

class Benchmark
  AB = 'ab'
  GO_AB = 'go-ab'
  PATTERN = /Requests per second:\s+([\d\.]+)\s+\[#\/sec\]/

  def initialize(opts)
    @url = opts[:url]
    @requests = opts[:requests]
    @min_concurrency = 1
    @max_concurrency = opts[:max_concurrency]
    @step = opts[:step]

    @concurrencies = (@min_concurrency..@max_concurrency).select { |n| n == 1 || (n % @step).zero? }
    @ab_results = []
    @go_ab_results = []
  end

  def run
    @concurrencies.each do |concurrency|
      STDERR.print "concurrency: #{concurrency}\r"
      @ab_results << do_request(AB, concurrency)
      @go_ab_results << do_request(GO_AB, concurrency)
    end
    STDERR.puts ""
  end

  def output
    puts ['concurrency', *@concurrencies].join("\t")
    puts [AB, *@ab_results].join("\t")
    puts [GO_AB, *@go_ab_results].join("\t")
  end

  private

  def do_request(cmd, concurrency)
    throughput = nil
    `#{cmd} -q -n #{@requests} -c #{concurrency} #{@url}`.match(PATTERN) { |m| throughput = m[1] }
    throughput.to_f
  end
end

STDOUT.sync = true

option_parser = OptionParser.new
opts = {
  url: 'http://127.0.0.1:8000/',
  requests: 1_000,
  max_concurrency: 100,
  step: 10
}

option_parser.on('-u', '--url URL') { |v| opts[:url] = v }
option_parser.on('-n', '--requests Number of Requests') { |v| opts[:requests] = v.to_i }
option_parser.on('-c', '--concurrency Max concurrency') { |v| opts[:max_concurrency] = v.to_i }
option_parser.on('-s', '--step Number of increment step') { |v| opts[:step] = v.to_i }

option_parser.parse!(ARGV)

b = Benchmark.new(opts)
b.run
b.output
