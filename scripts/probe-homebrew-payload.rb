# Developer-only acceptance. Mutations are confined to a disposable macOS VM.
require "open3"
model, status = Open3.capture2("/usr/sbin/sysctl", "-n", "hw.model")
raise "disposable macOS VM required" unless status.success? && model.strip.start_with?("VirtualMac") && HOMEBREW_PREFIX.to_s == "/opt/homebrew"
require ARGV.fetch(1)
root = Pathname(ARGV.fetch(0)).realpath
raise "private VM workspace required" unless root.to_s.start_with?("/private/tmp/brewwarden-probe.")
results = []
names = ARGV.drop(2)
raise "explicit formula targets required" if names.empty? || names.any? { |name| !name.match?(/\A[a-z0-9][a-z0-9+@_.-]*\z/) }
names.each do |name|
  formula = Formulary.factory("homebrew/core/#{name}")
  formula.force_bottle = true
  formula.fetch_bottle_tab(quiet: true)
  prefix = HOMEBREW_CELLAR/name/formula.pkg_version.to_s
  before = BrewWardenPayload.inventory(prefix)
  digest = BrewWardenPayload.verify(formula, root)
  results << { name:, case: "unchanged payload", digest: }
  executable = prefix.find.select { |path| path.file? && !path.symlink? && path.executable? }.sort.first
  raise "executable fixture required" unless executable
  cases = {
    "modified executable" => [executable, :bytes],
    "unexpected file" => [prefix/"unexpected-payload", :extra],
    "missing file" => [prefix/".brew/#{name}.rb", :missing],
    "changed permissions" => [executable, :mode],
    "changed SBOM package" => [prefix/"sbom.spdx.json", :sbom],
    "missing receipt" => [prefix/"INSTALL_RECEIPT.json", :missing],
  }
  cases.each do |label, (path, kind)|
    bytes = path.exist? ? path.binread : nil
    mode = path.exist? ? path.stat.mode & 0o7777 : nil
    begin
      case kind
      when :bytes
        path.chmod(mode | 0o200)
        path.open("ab") { |file| file.write("\nchanged\n") }
        path.chmod(mode)
      when :extra then path.write("unexpected")
      when :missing then path.unlink
      when :mode then path.chmod(0o600)
      when :sbom
        doc = JSON.parse(path.read)
        doc.fetch("packages").fetch(0)["versionInfo"] = "0.0.0"
        path.write(JSON.pretty_generate(doc))
      end
      failure = nil
      begin
        BrewWardenPayload.verify(formula, root)
      rescue RuntimeError => error
        failure = error.message
      end
      raise "changed payload accepted: #{label}" unless failure && ["installed payload differs from verified bottle", "missing installed receipt"].include?(failure)
      results << { name:, case: label, rejected: failure }
    ensure
      if bytes
        path.chmod(mode | 0o200) if path.exist?
        path.binwrite(bytes)
        path.chmod(mode)
      else
        path.unlink if path.exist?
      end
    end
    raise "fixture restoration failed" unless BrewWardenPayload.inventory(prefix) == before
  end
  raise "final payload changed" unless BrewWardenPayload.verify(formula, root) == digest
end
puts JSON.pretty_generate({ schema: 1, results: })
