# Developer-only acceptance probes for fixed official bottle closures.
# The shell driver verifies provenance and freezes inputs before invoking this file.
require "api"
require "json"
require "digest"
require "formulary"

root = Pathname(ARGV.fetch(0))
mode = ARGV.fetch(1)
scenario = ARGV.fetch(2, "hello")
# [version, rebuild, runtime dependencies, build-only dependencies]
scenarios = {
  "hello" => { "hello" => ["2.12.3", 1, [], []] },
  "jq" => { "jq" => ["1.8.2", 1, ["oniguruma"], []],
            "oniguruma" => ["6.9.10", 0, [], %w[autoconf automake libtool]] },
}
plan = scenarios.fetch(scenario)
recipe_names = plan.keys + (scenario == "hello" ? ["texinfo"] : %w[autoconf automake libtool m4])
inputs = root/"inputs"
raise "unsupported platform" unless Hardware::CPU.arm? && MacOS.version.to_sym == :tahoe
raise "metadata exceeds 80 MiB" if (root/"formula.jws.json").size > 80 * 1024 * 1024
verified, payload = Homebrew::API.send(:verify_and_parse_jws, JSON.parse((root/"formula.jws.json").read))
raise "metadata signature failed" unless verified == true
metadata = {}
recipe_names.each do |name|
  matches = payload.select { |entry| entry["name"] == name }
  raise "ambiguous formula" unless matches.length == 1
  entry = matches.first
  metadata[name] = entry
  relative = entry.fetch("ruby_source_path")
  directory = name.start_with?("lib") ? "lib" : name[0]
  raise "unexpected recipe path" unless relative == "Formula/#{directory}/#{name}.rb"
  expected_recipe = entry.fetch("ruby_source_checksum").fetch("sha256")
  snapshot = HOMEBREW_LIBRARY/"Taps/homebrew/homebrew-core"/relative
  raise "snapshot digest mismatch" unless [snapshot, inputs/"#{name}.rb"].all? { |p| Digest::SHA256.file(p).hexdigest == expected_recipe }
end

artifacts = {}
plan.each do |name, (version, rebuild, runtime_deps, build_deps)|
  entry = metadata.fetch(name)
  raise "unexpected version" unless entry.fetch("versions").fetch("stable") == version && entry.fetch("revision") == 0
  raise "unexpected rebuild" unless entry.fetch("bottle").fetch("stable").fetch("rebuild") == rebuild
  raise "unexpected dependencies" unless entry.fetch("dependencies").sort == runtime_deps && entry.fetch("build_dependencies").sort == build_deps
  selected = entry.fetch("bottle").fetch("stable").fetch("files").fetch("arm64_tahoe")
  raise "relocation unsupported" unless [":any", ":any_skip_relocation"].include?(selected.fetch("cellar"))
  filename = "#{name}--#{version}.arm64_tahoe.bottle#{rebuild.positive? ? ".#{rebuild}" : ""}.tar.gz"
  bottle = inputs/filename
  expected = selected.fetch("sha256")
  raise "bottle digest mismatch" unless Digest::SHA256.file(bottle).hexdigest == expected
  results = JSON.parse((inputs/"#{name}-verified-attestation.json").read)
  workflow = name == "oniguruma" ? "dispatch-build-bottle" : "publish-commit-bottles"
  identity = "https://github.com/Homebrew/homebrew-core/.github/workflows/#{workflow}.yml@refs/heads/main"
  raise "missing verified provenance" unless results.any? do |result|
    verification = result.fetch("verificationResult")
    certificate = verification.fetch("signature").fetch("certificate")
    certificate.fetch("subjectAlternativeName") == identity &&
      certificate.fetch("issuer") == "https://token.actions.githubusercontent.com" &&
      certificate.fetch("sourceRepositoryURI") == "https://github.com/Homebrew/homebrew-core" &&
      certificate.fetch("runnerEnvironment") == "github-hosted" &&
      verification.fetch("statement").fetch("subject").any? do |subject|
        subject.fetch("name") == filename && subject.fetch("digest").fetch("sha256") == expected
      end
  end
  artifacts[name] = { path: bottle, sha256: expected }
end

if mode == "inputs"
  puts "Authenticated all bottles, recipe snapshots and verified provenance before recipe loading."
  exit
end

records = []
cached_paths = []
plan.each do |name, (version, _rebuild, runtime_deps, build_deps)|
  artifact = artifacts.fetch(name)
  bottle = artifact.fetch(:path)
  # FromBottleLoader can fall back to the current recipe. Detect that fallback
  # rather than treating it as proof of the embedded recipe's dependency graph.
  embedded_formula = Formulary.factory(bottle.to_s)
  formula = Formulary.factory("homebrew/core/#{name}")
  embedded = Utils::Bottles.formula_contents(bottle, name:)
  raise "unexpected formula" unless [formula, embedded_formula].all? { |f| f.name == name && f.pkg_version.to_s == version }
  raise "not loaded from bottle" unless embedded_formula.local_bottle_path == bottle.realpath && embedded_formula.path.to_s.end_with?("/Cellar/#{name}/#{version}/.brew/#{name}.rb")
  snapshot = HOMEBREW_LIBRARY/"Taps/homebrew/homebrew-core"/metadata.fetch(name).fetch("ruby_source_path")
  raise "recipe snapshot not consumed" unless formula.path.realpath == snapshot.realpath
  [formula, embedded_formula].each do |f|
    raise "requirements unsupported" unless f.requirements.empty?
    raise "post-install unsupported" if f.post_install_defined?
    raise "test or optional dependency unsupported" if f.deps.any? { |d| d.test? || d.optional? || d.recommended? }
    raise "runtime graph mismatch" unless f.deps.reject(&:build?).map(&:name).sort == runtime_deps
    raise "build graph mismatch" unless f.deps.select(&:build?).map(&:name).sort == build_deps
  end
  if mode != "after"
    formula.fetch_bottle_tab
    manifest_deps = formula.bottle_tab_attributes.fetch("runtime_dependencies")
    expected_deps = runtime_deps.map { |dep| [dep, plan.fetch(dep).first, 0] }
    actual_deps = manifest_deps.map { |d| [d.fetch("full_name"), d.fetch("version"), d.fetch("revision")] }
    raise "bottle runtime graph mismatch" unless actual_deps.sort == expected_deps.sort
  end
  raise "cache digest mismatch" unless Digest::SHA256.file(formula.bottle.cached_download).hexdigest == artifact.fetch(:sha256)
  cached_paths << formula.bottle.cached_download.realpath.to_s
  if mode == "after"
    raise "unexpected installed version" unless (HOMEBREW_CELLAR/name).children.map { |p| p.basename.to_s } == [version]
    raise "installed recipe differs" unless (formula.prefix/".brew/#{name}.rb").read == embedded
    receipt = JSON.parse((formula.prefix/"INSTALL_RECEIPT.json").read)
    raise "source fallback" unless receipt.fetch("poured_from_bottle") == true
    installed_deps = receipt.fetch("runtime_dependencies").map { |d| [d.fetch("full_name"), d.fetch("version"), d.fetch("revision")] }
    raise "installed dependency graph mismatch" unless installed_deps.sort == runtime_deps.map { |dep| [dep, plan.fetch(dep).first, 0] }.sort
  end
  records << { name:, version:, bottleSHA256: artifact.fetch(:sha256),
               embeddedRecipeSHA256: Digest::SHA256.hexdigest(embedded), dependencies: runtime_deps }
end

if %w[before inspect].include?(mode)
  raise "prefix is not empty" unless !HOMEBREW_CELLAR.exist? || HOMEBREW_CELLAR.children.empty?
  # Freeze every existing cache input before installation can re-resolve it.
  # The recipes and bottles are immutable too; no online fallback is possible.
  cache_inputs = Dir.glob((HOMEBREW_CACHE/"**/*").to_s).select { |p| File.file?(p) || File.symlink?(p) }
  File.open(root/"sandbox.sb", "a") do |profile|
    cache_inputs.each do |path|
      raise "unsafe cache input path" unless path.match?(%r{\A/[-a-zA-Z0-9_./]+\z}) && File.realpath(path).start_with?(root.to_s + "/")
      profile.puts "(deny file-write* (literal #{path.to_json}))" if mode == "before"
    end
  end
  (root/"cached-bottle-path").write(cached_paths.last)
elsif mode == "after"
  raise "unexpected installed closure" unless HOMEBREW_CELLAR.children.map { |p| p.basename.to_s }.sort == plan.keys.sort
else
  raise "unknown phase"
end
puts JSON.pretty_generate({ phase: mode, target: scenario, closure: records })
