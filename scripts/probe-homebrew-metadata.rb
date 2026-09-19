# Developer-only probe: invoked by the isolated Homebrew ruby command.
# This exercises the upstream verifier with its actual bundled trust root.
require "api"
require "json"
require "digest"

path = ARGV.fetch(0)
raise "metadata exceeds 80 MiB" if File.size(path) > 80 * 1024 * 1024
raw = File.binread(path)
document = JSON.parse(raw)
verified, payload = Homebrew::API.send(:verify_and_parse_jws, document)
raise "official metadata did not verify" unless verified == true
raise "unexpected formula payload" unless payload.is_a?(Array)

changed = JSON.parse(raw)
changed["payload"] += " "
accepted, = Homebrew::API.send(:verify_and_parse_jws, changed)
raise "changed signed bytes accepted" unless accepted == false

unsigned = JSON.parse(raw)
unsigned["signatures"] = []
accepted, = Homebrew::API.send(:verify_and_parse_jws, unsigned)
raise "missing signature accepted" unless accepted == false

formula = payload.find { |entry| entry["name"] == "wget" }
raise "representative formula missing" unless formula
puts JSON.pretty_generate({
  "metadataSHA256" => Digest::SHA256.hexdigest(raw),
  "verifiedWithUpstreamTrustRoot" => true,
  "changedPayloadRejected" => true,
  "missingSignatureRejected" => true,
  "formulaCount" => payload.length,
  "sample" => {
    "name" => formula["name"],
    "versions" => formula["versions"],
    "revision" => formula["revision"],
    "bottle" => formula["bottle"],
    "timeRelatedTopLevelFields" => formula.keys.grep(/date|time|publish|release/),
  },
})
