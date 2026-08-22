require "json"
require_relative "dep1"

def run
  puts JSON.generate(Dep1.example_text)
end

run
