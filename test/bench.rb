#!/usr/bin/env ruby

class Benchmark
  AB = 'ab'
  GO_AB = 'go-ab'
  PATTERN = /Requests per second:\s+([\d\.]+)\s+\[#\/sec\]/

  def initialize
    @url = 'http://127.0.0.1:8000/'
    @requests = 1_000
    @min_concurrency = 1
    @max_concurrency = 100
    @step = 10

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

b = Benchmark.new
b.run
b.output
