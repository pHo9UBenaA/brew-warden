# Native JWS verification precedes all interpretation of authenticated recipes.
require "api"
require "json"
require "digest"
root = Pathname(ARGV.shift)
raise "unsupported platform" unless Hardware::CPU.arm? && MacOS.version.to_sym == :tahoe
raise "metadata exceeds limit" if (root/"formula.jws.json").size > 80 * 1024 * 1024
verified, payload = Homebrew::API.send(:verify_and_parse_jws, JSON.parse((root/"formula.jws.json").read))
raise "metadata signature failed" unless verified == true && payload.is_a?(Array)
index = {}
payload.each do |entry|
  name = entry.fetch("name")
  raise "ambiguous formula" if index.key?(name)
  index[name] = entry
end
pending = ARGV.dup
raise "empty request" if pending.empty?
selected = {}
until pending.empty?
  name = pending.shift
  raise "invalid formula name" unless name.match?(/\A[a-z0-9][a-z0-9+_.@-]{0,127}\z/)
  next if selected.key?(name)
  raise "formula inventory exceeds limit" if selected.length >= 128
  entry = index.fetch(name)
  path = entry.fetch("ruby_source_path")
  directory = name.start_with?("lib") ? "lib" : name[0]
  raise "unsupported recipe path" unless path == "Formula/#{directory}/#{name}.rb"
  dependencies = entry.fetch("dependencies")
  build_dependencies = entry.fetch("build_dependencies")
  raise "invalid dependencies" unless dependencies.is_a?(Array) && build_dependencies.is_a?(Array)
  bottle = entry.fetch("bottle").fetch("stable")
  chosen = bottle.fetch("files")["arm64_tahoe"]
  selected[name] = {
    name:, version: entry.fetch("versions").fetch("stable"), revision: entry.fetch("revision"),
    rebuild: bottle.fetch("rebuild"), sourceURL: entry.fetch("urls").fetch("stable").fetch("url"),
    sourceSHA256: entry.fetch("urls").fetch("stable").fetch("checksum"),
    recipeSHA256: entry.fetch("ruby_source_checksum").fetch("sha256"),
    recipePath: path, tapCommit: entry.fetch("tap_git_head"),
    bottleURL: chosen ? chosen.fetch("url") : "", bottleSHA256: chosen ? chosen.fetch("sha256") : "",
    cellar: chosen ? chosen.fetch("cellar") : "", dependencies:, buildDependencies: build_dependencies,
  }
  pending.concat(dependencies + build_dependencies)
end
puts JSON.generate({ schema: 1, platform: "arm64_tahoe", formulae: selected.values.sort_by { |entry| entry.fetch(:name) } })
