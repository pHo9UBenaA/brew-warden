require_relative "candidate"
output = STDOUT.dup
STDOUT.reopen(STDERR)
root = Pathname(ARGV.fetch(0))
candidate = BrewWardenCandidate.new(root)
output.puts JSON.generate({ schema: 1, candidates: candidate.validate })
