# Developer-only acceptance probe for one dependency-free, relocatable core bottle.
# The shell driver verifies provenance and freezes inputs before invoking this file.
require "api"
require "json"
require "digest"
require "formulary"

root = Pathname(ARGV.fetch(0))
mode = ARGV.fetch(1)
inputs = root/"inputs"
bottle = inputs/"hello--2.12.3.arm64_tahoe.bottle.1.tar.gz"
raise "unsupported platform" unless Hardware::CPU.arm? && MacOS.version.to_sym == :tahoe
verified, payload = Homebrew::API.send(:verify_and_parse_jws, JSON.parse((root/"formula.jws.json").read))
raise "metadata signature failed" unless verified == true
matches = payload.select { |entry| entry["name"] == "hello" }
raise "ambiguous formula" unless matches.length == 1
entry = matches.fetch(0)
raise "unexpected version" unless entry.fetch("versions").fetch("stable") == "2.12.3" && entry.fetch("revision") == 0
raise "unexpected rebuild" unless entry.fetch("bottle").fetch("stable").fetch("rebuild") == 1
raise "unexpected dependencies" unless entry.fetch("dependencies") == [] && entry.fetch("build_dependencies") == []
selected = entry.fetch("bottle").fetch("stable").fetch("files").fetch("arm64_tahoe")
raise "relocation unsupported" unless selected.fetch("cellar") == ":any_skip_relocation"
expected = selected.fetch("sha256")
raise "bottle digest mismatch" unless Digest::SHA256.file(bottle).hexdigest == expected
raise "recipe digest mismatch" unless Digest::SHA256.file(inputs/"hello.rb").hexdigest == entry.fetch("ruby_source_checksum").fetch("sha256")
%w[hello texinfo].each do |name|
  metadata = payload.select { |item| item["name"] == name }
  raise "ambiguous recipe" unless metadata.length == 1
  relative = metadata.first.fetch("ruby_source_path")
  expected_recipe = metadata.first.fetch("ruby_source_checksum").fetch("sha256")
  raise "unexpected recipe path" unless relative == "Formula/#{name[0]}/#{name}.rb"
  snapshot = HOMEBREW_LIBRARY/"Taps/homebrew/homebrew-core"/relative
  raise "snapshot digest mismatch" unless Digest::SHA256.file(snapshot).hexdigest == expected_recipe
end
results = JSON.parse((inputs/"verified-attestation.json").read)
identity = "https://github.com/Homebrew/homebrew-core/.github/workflows/publish-commit-bottles.yml@refs/heads/main"
raise "missing verified provenance" unless results.any? do |result|
  verification = result.fetch("verificationResult")
  certificate = verification.fetch("signature").fetch("certificate")
  certificate.fetch("subjectAlternativeName") == identity &&
    certificate.fetch("issuer") == "https://token.actions.githubusercontent.com" &&
    certificate.fetch("sourceRepositoryURI") == "https://github.com/Homebrew/homebrew-core" &&
    certificate.fetch("runnerEnvironment") == "github-hosted" &&
    verification.fetch("statement").fetch("subject").any? do |subject|
      subject.fetch("name") == bottle.basename.to_s && subject.fetch("digest").fetch("sha256") == expected
    end
end

if mode == "inputs"
  puts "Authenticated bottle, recipe snapshots and verified provenance before recipe loading."
  exit
end

# FromBottleLoader can fall back to the current recipe. Require the embedded
# recipe path and bytes so that a fallback cannot pass this acceptance probe.
embedded_formula = Formulary.factory(bottle.to_s)
formula = Formulary.factory("homebrew/core/hello")
embedded = Utils::Bottles.formula_contents(bottle, name: "hello")
raise "unexpected formula" unless formula.name == "hello" && formula.pkg_version.to_s == "2.12.3"
raise "not loaded from bottle" unless embedded_formula.local_bottle_path == bottle.realpath && embedded_formula.path.to_s.include?("/Cellar/hello/2.12.3/.brew/hello.rb")
raise "recipe snapshot not consumed" unless formula.path.realpath == (HOMEBREW_LIBRARY/"Taps/homebrew/homebrew-core/Formula/h/hello.rb").realpath
raise "dependency closure not empty" unless [formula, embedded_formula].all? { |f| f.deps.empty? && f.requirements.empty? }
raise "post-install unsupported" if embedded_formula.post_install_defined?
if mode == "before"
  formula.fetch_bottle_tab
  raise "bottle runtime closure not empty" unless formula.bottle_tab_attributes.fetch("runtime_dependencies") == []
end
raise "cache digest mismatch" unless Digest::SHA256.file(formula.bottle.cached_download).hexdigest == expected
raise "post-install unsupported" if formula.post_install_defined?

if mode == "before"
  raise "prefix is not empty" unless !HOMEBREW_CELLAR.exist? || HOMEBREW_CELLAR.children.empty?
  # Freeze every existing cache input before installation can re-resolve it.
  # The source and bottle are immutable too; no online fallback is possible.
  cache_inputs = Dir.glob((HOMEBREW_CACHE/"**/*").to_s).select { |p| File.file?(p) || File.symlink?(p) }
  File.open(root/"sandbox.sb", "a") do |profile|
    cache_inputs.each do |path|
      raise "unsafe cache input path" unless path.match?(%r{\A/[-a-zA-Z0-9_./]+\z}) && File.realpath(path).start_with?(root.to_s + "/")
      profile.puts "(deny file-write* (literal #{path.to_json}))"
    end
  end
  (root/"cached-bottle-path").write(formula.bottle.cached_download.realpath.to_s)
  (root/"embedded-recipe.sha256").write(Digest::SHA256.hexdigest(embedded) + "\n")
elsif mode == "after"
  raise "unexpected installed closure" unless HOMEBREW_CELLAR.children.map { |p| p.basename.to_s }.sort == ["hello"]
  raise "unexpected installed version" unless (HOMEBREW_CELLAR/"hello").children.map { |p| p.basename.to_s } == ["2.12.3"]
  raise "installed recipe differs" unless (formula.prefix/".brew/hello.rb").read == embedded
  receipt = JSON.parse((formula.prefix/"INSTALL_RECEIPT.json").read)
  raise "source fallback" unless receipt.fetch("poured_from_bottle") == true
  raise "unexpected runtime closure" unless receipt.fetch("runtime_dependencies") == []
else
  raise "unknown phase"
end
puts JSON.pretty_generate({ phase: mode, name: formula.name, version: formula.pkg_version.to_s,
  bottleSHA256: expected, embeddedRecipeSHA256: Digest::SHA256.hexdigest(embedded), dependencies: [] })
