# Reject any native source-build fallback, including paths that --force-bottle
# alone does not prohibit. The reviewed plan supplies every installer identity.
module BrewWardenInstallerGuard
  def initialize(formula, **options)
    session = $brewwarden_session
    raise "unplanned native installer" unless session && session.actions.any? { |action| action[:name] == formula.name && action[:operation] != "keep" } && session.candidate.formulae.fetch(formula.name).path == formula.path
    super
  end
  def build
    raise "native source fallback prohibited"
  end
end

