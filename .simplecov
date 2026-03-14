# bashcov / simplecov configuration for nself CLI
# Used by: bashcov --skip-uncovered -- bats src/tests/bats/
# CI workflow: .github/workflows/coverage.yml

require 'simplecov-cobertura'

SimpleCov.start do
  add_filter '/tests/'
  add_filter '/src/templates/'
  add_filter '/.releases/'

  minimum_coverage 80
  minimum_coverage_by_file 0

  coverage_dir 'coverage'

  formatter SimpleCov::Formatter::MultiFormatter.new([
    SimpleCov::Formatter::HTMLFormatter,
    SimpleCov::Formatter::CoberturaFormatter,
  ])
end
