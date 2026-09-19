# Developer-only contract probes against the actual pinned Homebrew evaluator.
require "vulns/advisory_database"
require "json"
require "digest"

root = Pathname(ARGV.fetch(0))
record = {
  "id" => "BREW-PROBE-REVISION",
  "affected" => [{
    "package" => { "ecosystem" => "Homebrew", "name" => "probe" },
    "ranges" => [{ "type" => "ECOSYSTEM", "events" => [{ "introduced" => "0" }, { "fixed" => "2.12.3_1" }] }],
    "ecosystem_specific" => { "fix" => "patch" },
  }],
}
data = { "advisories" => { "probe" => [record] } }
database = Homebrew::Vulns::AdvisoryDatabase.new(data)
original = database.status_for("probe", "2.12.3")
patched = database.status_for("probe", "2.12.3_1")
raise "unpatched revision not affected" unless original.fetch("open").length == 1
raise "patched revision not distinguished" unless patched.fetch("open").empty? && patched.fetch("patched").length == 1
raise "uncovered package reported clean" unless database.status_for("uncovered", "1.0").nil?

# The native wrapper does not handle withdrawal itself. The product adapter
# must inspect withdrawals before using this lower-level status method.
withdrawn = JSON.parse(JSON.generate(record))
withdrawn["withdrawn"] = "2026-01-01T00:00:00Z"
withdrawn_status = Homebrew::Vulns::AdvisoryDatabase.new({ "advisories" => { "probe" => [withdrawn] } }).status_for("probe", "2.12.3")
raise "revisit withdrawal capability" unless withdrawn_status.fetch("open").length == 1

incomplete = { "id" => "BREW-PROBE-INCOMPLETE", "affected" => [] }
incomplete_status = Homebrew::Vulns::AdvisoryDatabase.new({ "advisories" => { "probe" => [incomplete] } }).status_for("probe", "2.12.3")
raise "revisit incomplete range capability" unless incomplete_status.fetch("open").length == 1

# Exercise actual cache refresh failure under the driver's network-denying
# sandbox. The inherited loader returns stale data despite the refresh failure.
class ProbeFeed < Homebrew::Vulns::AdvisoryDatabase
  def self.data_url = "https://example.invalid/brewwarden-advisories.json"
end
cache = root/"advisory-cache"
cache.mkpath
file = cache/"advisories.json"
file.write(JSON.generate(data))
old = Time.now - 172_800
File.utime(old, old, file)
stale = ProbeFeed.load(cache:, max_age: 1)
raise "revisit stale fallback capability" unless stale.status_for("probe", "2.12.3") == original
future = Time.now + 86_400
File.utime(future, future, file)
future_dated = ProbeFeed.load(cache:, max_age: 1)
raise "revisit future cache capability" unless future_dated.status_for("probe", "2.12.3") == original

result = { revisionPatchDistinguished: true, uncoveredIsUnknown: true,
  withdrawnStillOpen: true, incompleteRangeStillOpen: true,
  staleFallbackReturned: true, futureCacheAccepted: true }
if ARGV[1]
  path = Pathname(ARGV[1])
  raise "feed exceeds 64 MiB" if path.size > 64 * 1024 * 1024
  live = Homebrew::Vulns::AdvisoryDatabase.new(JSON.parse(path.read))
  result[:sample] = { sha256: Digest::SHA256.file(path).hexdigest,
    formulaCount: live.formulae.length, meta: live.meta, hello: live.status_for("hello", "2.12.3") }
end
puts JSON.pretty_generate(result)
