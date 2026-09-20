require_relative "candidate"
require_relative "payload"
require_relative "installer_guard"
require "install"
require "upgrade"
require "lock_file"

# All protocol files live in the private workspace; no user text is executable.
class BrewWardenSession
  attr_reader :root, :candidate, :actions
  def initialize(root)
    @root = root
    @candidate = BrewWardenCandidate.new(root)
    @actions = []
    @locks = []
    @deadline = Process.clock_gettime(Process::CLOCK_MONOTONIC) + 180
  end

  def emit(sequence, document)
    path = root/"control/event-#{sequence}.json"
    temporary = Pathname("#{path}.pending")
    temporary.open(File::WRONLY | File::CREAT | File::EXCL, 0o600) { |file| file.write(JSON.generate(document)); file.fsync }
    temporary.rename(path)
    path.dirname.open { |directory| directory.fsync }
  end

  def installed
    return [] unless HOMEBREW_CELLAR.directory?
    entries = HOMEBREW_CELLAR.children.sort
    raise "installed inventory exceeds limit" if entries.length > 1024
    entries.each do |entry|
      raise "unsupported installed rack" unless entry.directory? && !entry.symlink? && entry.basename.to_s.match?(/\A[a-z0-9][a-z0-9+_.@-]{0,127}\z/)
    end
    entries.map { |entry| entry.basename.to_s }
  end

  def state
    records = []
    installed.each do |name|
      rack = HOMEBREW_CELLAR/name
      kegs = rack.children.sort
      raise "installed version inventory exceeds limit" if kegs.length > 32
      kegs.each do |keg|
        raise "invalid installed keg" unless keg.directory? && !keg.symlink? && keg.realpath == keg && keg.basename.to_s.match?(/\A[A-Za-z0-9][A-Za-z0-9+_.:-]{0,127}\z/)
        receipt = keg/"INSTALL_RECEIPT.json"
        raise "unsupported installed receipt" unless receipt.file? && !receipt.symlink? && receipt.size <= 1024 * 1024
        tab = JSON.parse(receipt.read)
        dependencies = tab.fetch("runtime_dependencies")
        raise "incomplete installed dependencies" unless dependencies.is_a?(Array) && dependencies.length <= 128
        dependencies.each do |dep|
          full = dep.fetch("full_name")
          raise "invalid installed dependency" unless full.is_a?(String) && full.match?(/\A(?:[a-zA-Z0-9_.-]+\/[a-zA-Z0-9_.-]+\/)?[a-z0-9][a-z0-9+_.@-]{0,127}\z/)
        end
        record = { name:, version: keg.basename.to_s, receipt: Digest::SHA256.file(receipt).hexdigest, dependencies: }
        record[:payload] = BrewWardenPayload.inventory(keg) if candidate.items.key?(name)
        records << record
      end
      opt = HOMEBREW_PREFIX/"opt"/name
      raise "unsupported active keg" if opt.exist? && !opt.symlink?
      linked = HOMEBREW_LINKED_KEGS/name
      raise "unsupported linked keg record" if linked.exist? && !linked.symlink?
      records << { name:, opt: opt.symlink? ? opt.readlink.to_s : nil, linked: linked.symlink? ? linked.readlink.to_s : nil }
    end
    [BrewWardenPayload.store_snapshot(root, records), records]
  end

  def plan
    @actions = candidate.items.keys.sort.map do |name|
      formula = candidate.formulae.fetch(name)
      expected = HOMEBREW_CELLAR/name/formula.pkg_version.to_s
      active = formula.opt_prefix
      raise "pinned candidate" if formula.pinned?
      operation = if expected.directory?
        candidate.match_installed([name])
        BrewWardenPayload.verify(formula, root)
        "keep"
      elsif active.symlink?
        old = active.realpath
        raise "active keg outside candidate rack" unless old.parent == HOMEBREW_CELLAR/name && old.directory? && !old.symlink?
        raise "candidate downgrade is unsupported" unless PkgVersion.parse(old.basename.to_s) < formula.pkg_version
        receipt = old/"INSTALL_RECEIPT.json"
        raise "unsupported installed receipt" unless receipt.file? && !receipt.symlink? && receipt.size <= 1024 * 1024
        tab = JSON.parse(receipt.read)
        raise "installed candidate is not official core stable" unless tab.fetch("source").fetch("tap") == "homebrew/core" && tab.fetch("source").fetch("spec") == "stable" && tab.fetch("arch") == "arm64" && tab.fetch("used_options").empty?
        "upgrade"
      else
        raise "unlinked existing keg requires reconciliation" if (HOMEBREW_CELLAR/name).directory? && !(HOMEBREW_CELLAR/name).children.empty?
        raise "upgrade target is not installed" if candidate.document.fetch("operation") == "upgrade" && candidate.document.fetch("targets").include?(name)
        "install"
      end
      { name:, operation: }
    end
    changed = actions.reject { |action| action[:operation] == "keep" }.map { |action| action[:name] }
    _, inventory = state
    inventory.each do |record|
      next if candidate.items.key?(record[:name]) || !record[:dependencies]
      if record[:dependencies].any? { |dep| changed.include?(dep.fetch("full_name").split("/").last) }
        raise "affected installed dependent requires a verified plan"
      end
    end
    actions.reject { |action| action[:operation] == "keep" }.each do |action|
      installer = FormulaInstaller.new(candidate.formulae.fetch(action[:name]), force_bottle: true)
      installer.determine_bottle_tab_attributes
      installer.check_install_sanity
      raise "native source fallback prohibited" unless installer.pour_bottle?
      raise "unplanned native dependency" unless installer.compute_dependencies.all? { |dep| candidate.items.key?(dep.name) }
    end
  end

  def validate_seal
    file = root/"seal.json"
    raise "invalid session seal" unless file.file? && !file.symlink? && file.size <= 8 * 1024 * 1024
    seal = JSON.parse(file.read)
    raise "invalid session seal" unless seal.keys.sort == %w[beforeState expiresAt inputs issuedAt plan schema] && seal.fetch("schema") == 1
    raise "plan expired or clock changed" unless seal.fetch("expiresAt").is_a?(Integer) && seal.fetch("issuedAt").is_a?(Integer) && Time.now.to_i.between?(seal.fetch("issuedAt"), seal.fetch("expiresAt") - 1)
    raise "plan changed" unless seal.fetch("plan").match?(/\A[0-9a-f]{64}\z/) && Digest::SHA256.file(root/"plan.json").hexdigest == seal.fetch("plan")
    entries = seal.fetch("inputs")
    raise "invalid frozen input inventory" unless entries.is_a?(Array) && !entries.empty? && entries.length <= 10000
    entries.each do |entry|
      path = root/entry.fetch("path")
      raise "invalid frozen input path" unless path.file? && !path.symlink? && path.realpath.to_s.start_with?(root.to_s + "/")
      raise "frozen input changed" unless Digest::SHA256.file(path).hexdigest == entry.fetch("sha256")
    end
    raise "installed state changed" unless state[0] == seal.fetch("beforeState")
    candidate.validate
    seal
  end

  def execute(seal)
    remaining = actions.reject { |action| action[:operation] == "keep" }.map { |action| action[:name] }
    until remaining.empty?
      name = remaining.find { |item| (candidate.items.fetch(item).fetch("dependencies") & remaining).empty? }
      raise "cyclic native plan" unless name
      raise "plan expired during execution" unless Time.now.to_i.between?(seal.fetch("issuedAt"), seal.fetch("expiresAt") - 1)
      formula = candidate.formulae.fetch(name)
      action = actions.find { |item| item[:name] == name }
      installer = FormulaInstaller.new(formula, force_bottle: true, installed_on_request: candidate.document.fetch("targets").include?(name))
      installer.determine_bottle_tab_attributes
      raise "unexpected native dependency action" unless installer.compute_dependencies.all? { |dep| candidate.items.key?(dep.name) }
      raise "native source fallback prohibited" unless installer.pour_bottle?
      # All runtime dependencies are already installed in topological order.
      # Keep native dependency checks enabled; an unexpected installer is blocked.
      installer.prelude
      raise "plan expired during prelude" unless Time.now.to_i.between?(seal.fetch("issuedAt"), seal.fetch("expiresAt") - 1)
      Homebrew::Install.install_formula(installer, upgrade: action[:operation] == "upgrade")
      raise "native installation failed" if Homebrew.failed?
      candidate.match_installed([name])
      BrewWardenPayload.verify(formula, root)
      remaining.delete(name)
    end
  end

  def run
    names = (installed + candidate.items.keys).uniq.sort
    raise "unexpected native installer locks" unless FormulaInstaller.locked.empty?
    names.each do |name|
      lock = FormulaLock.new(name)
      lock.lock
      @locks << lock
    end
    FormulaInstaller.locked.concat(candidate.formulae.values)
    raise "installed inventory changed while locking" unless (installed - names).empty?
    candidate.validate
    plan
    before = state[0]
    emit(0, { schema: 1, beforeState: before, actions:, osVersion: MacOS.full_version.to_s })
    sequence = 1
    loop do
      file = root/"control/command-#{sequence}.json"
      until file.exist?
        raise "idle session expired" if Process.clock_gettime(Process::CLOCK_MONOTONIC) >= @deadline
        sleep 0.05
      end
      raise "invalid native command" unless file.file? && !file.symlink? && file.size <= 4096
      command = JSON.parse(file.read)
      raise "invalid native command" unless command.keys.sort == %w[operation plan schema] && command.fetch("schema") == 1
      seal = validate_seal
      raise "command plan mismatch" unless command.fetch("plan") == seal.fetch("plan") && seal.fetch("beforeState") == before
      case command.fetch("operation")
      when "revalidate"
        emit(sequence, { schema: 1, beforeState: before })
      when "execute"
        code = 1
        matches = false
        begin
          execute(seal)
          candidate.match_installed
          code = 0
          matches = true
        rescue StandardError => error
          warn "BrewWarden native execution stopped: #{error.class}: #{error.message.to_s.byteslice(0, 1024).scrub.dump}"
        end
        after = state[0] rescue ""
        emit(sequence, { schema: 1, exitCode: code, afterState: after, matchesPlan: matches })
        return code
      else
        raise "unsupported native command"
      end
      sequence += 1
      raise "native command limit" if sequence > 4
    end
  ensure
    FormulaInstaller.locked.clear
    @locks.reverse_each(&:unlock)
  end
end


root = Pathname(ARGV.fetch(0)).realpath
raise "unsupported installation prefix" unless HOMEBREW_PREFIX.to_s == "/opt/homebrew"
$brewwarden_session = BrewWardenSession.new(root)
FormulaInstaller.prepend(BrewWardenInstallerGuard)
exit $brewwarden_session.run
