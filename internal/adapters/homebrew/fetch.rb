# Only authenticated recipes are evaluated. No installation is performed here.
require_relative "candidate"
output = STDOUT.dup
STDOUT.reopen(STDERR)
candidate = BrewWardenCandidate.new(Pathname(ARGV.fetch(0)))
downloads = candidate.items.keys.sort.map do |name|
  formula = candidate.formulae.fetch(name)
  bottle = formula.bottle
  raise "candidate bottle changed" unless bottle && bottle.resource.checksum.hexdigest == candidate.items.fetch(name).fetch("bottleSHA256")
  bottle.fetch(verify_download_integrity: true, timeout: 60, quiet: true)
  formula.fetch_bottle_tab(quiet: true)
  { name:, path: bottle.cached_download.realpath.to_s }
end
output.puts JSON.generate({ schema: 1, downloads: })
