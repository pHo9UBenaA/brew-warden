# Shared native candidate inspection. Never called on unauthenticated recipes or
# bottles. The Go adapter freezes these inputs before creating a session.
require "json"
require "digest"
require "formulary"
require_relative "source"

class BrewWardenCandidate
  attr_reader :root, :document, :formulae, :items

  def initialize(root)
    @root = root
    @document = JSON.parse((root/"native-inputs.json").read)
    raise "unsupported native plan" unless document.fetch("schema") == 1
    @items = document.fetch("candidates").to_h { |item| [item.fetch("name"), item] }
    raise "empty native plan" if items.empty? || items.length > 128
    document.fetch("recipes").each do |item|
      file = HOMEBREW_LIBRARY/"Taps/homebrew/homebrew-core"/item.fetch("recipePath")
      raise "recipe changed" unless file.file? && !file.symlink? && Digest::SHA256.file(file).hexdigest == item.fetch("recipeSHA256")
    end
    @formulae = items.keys.sort.to_h do |name|
      formula = Formulary.factory("homebrew/core/#{name}")
      formula.force_bottle = true
      [name, formula]
    end
  end

  def filename(item)
    version = item.fetch("version")
    version += "_#{item.fetch('revision')}" if item.fetch("revision").positive?
    rebuild = item.fetch("rebuild").positive? ? ".#{item.fetch('rebuild')}" : ""
    "#{item.fetch('name')}--#{version}.arm64_tahoe.bottle#{rebuild}.tar.gz"
  end

  def validate
    items.keys.sort.map do |name|
      item = items.fetch(name)
      formula = formulae.fetch(name)
      bottle = root/"inputs"/filename(item)
      raise "bottle changed" unless bottle.file? && !bottle.symlink? && Digest::SHA256.file(bottle).hexdigest == item.fetch("bottleSHA256")
      expected = HOMEBREW_LIBRARY/"Taps/homebrew/homebrew-core"/item.fetch("recipePath")
      raise "recipe snapshot not consumed" unless formula.path.realpath == expected.realpath
      raise "recipe changed" unless Digest::SHA256.file(expected).hexdigest == item.fetch("recipeSHA256")
      embedded_text = Utils::Bottles.formula_contents(bottle, name:)
      embedded = Formulary.factory(bottle.to_s)
      raise "embedded recipe fallback" unless embedded.local_bottle_path == bottle.realpath
      [formula, embedded].each do |candidate|
        raise "candidate identity mismatch" unless candidate.name == name && candidate.version.to_s == item.fetch("version") && candidate.revision == item.fetch("revision")
        raise "source identity mismatch" unless candidate.stable.url == item.fetch("sourceURL") && candidate.stable.checksum.hexdigest == item.fetch("sourceSHA256")
        raise "unsupported requirements or options" unless candidate.requirements.empty? && candidate.options.empty?
        validate_install_behavior(candidate)
        raise "unsupported migration or conflict" unless candidate.oldnames.empty? && candidate.conflicts.empty?
        raise "unsupported dependency kind" if candidate.deps.any? { |dep| dep.test? || dep.optional? || dep.recommended? }
        raise "runtime graph changed" unless candidate.deps.reject(&:build?).map(&:name).sort == item.fetch("dependencies").sort
        raise "build graph changed" unless candidate.deps.select(&:build?).map(&:name).sort == item.fetch("buildDependencies").sort
      end
      raise "native bottle identity mismatch" unless formula.bottle && formula.bottle.resource.checksum.hexdigest == item.fetch("bottleSHA256") && formula.bottle.rebuild == item.fetch("rebuild") && formula.bottle.tag.to_s == "arm64_tahoe"
      cached = formula.bottle.cached_download
      raise "cache outside workspace" unless cached.realpath.to_s.start_with?((root/"cache").to_s + "/")
      raise "cached bottle changed" unless Digest::SHA256.file(cached).hexdigest == item.fetch("bottleSHA256")
      formula.fetch_bottle_tab(quiet: true)
      dependencies = formula.bottle_tab_attributes.fetch("runtime_dependencies").map { |dep| [dep.fetch("full_name"), dep.fetch("version"), dep.fetch("revision")] }.sort
      wanted = item.fetch("dependencies").map { |dep| [dep, items.fetch(dep).fetch("version"), items.fetch(dep).fetch("revision")] }.sort
      raise "OCI dependency graph changed" unless dependencies == wanted
      embedded_sha = Digest::SHA256.hexdigest(embedded_text)
      { name:, cachePath: cached.realpath.to_s, embeddedRecipeSHA256: embedded_sha,
        unmodifiedSource: [formula, embedded].all? { |f| (f.class.instance_methods(false) - %i[install test]).empty? } &&
          BrewWardenSource.reviewed?(name, expected.read) && BrewWardenSource.reviewed?(name, embedded_text) }
    end
  end

  def validate_install_behavior(candidate)
    raise "unsupported post-install or service" if candidate.post_install_defined? || candidate.post_install_steps_defined? || candidate.class.service?
    raise "unsupported patches or resources" unless candidate.patchlist.empty? && candidate.resources.empty?
  end

  def match_installed
    items.each do |name, item|
      formula = formulae.fetch(name)
      prefix = HOMEBREW_CELLAR/name/formula.pkg_version.to_s
      raise "invalid candidate keg" unless prefix.directory? && !prefix.symlink? && prefix.realpath == prefix
      receipt = prefix/"INSTALL_RECEIPT.json"
      raise "candidate is not installed" unless receipt.file? && !receipt.symlink?
      tab = JSON.parse(receipt.read)
      raise "source fallback detected" unless tab.fetch("poured_from_bottle") == true
      dependencies = tab.fetch("runtime_dependencies").map { |dep| [dep.fetch("full_name"), dep.fetch("version"), dep.fetch("revision")] }.sort
      wanted = item.fetch("dependencies").map { |dep| [dep, items.fetch(dep).fetch("version"), items.fetch(dep).fetch("revision")] }.sort
      raise "installed runtime graph mismatch" unless dependencies == wanted
      embedded = Utils::Bottles.formula_contents(root/"inputs"/filename(item), name:)
      raise "installed recipe mismatch" unless (prefix/".brew/#{name}.rb").read == embedded
      raise "candidate not active" unless formula.opt_prefix.symlink? && formula.opt_prefix.realpath == prefix
    end
    true
  end
end
