module Reporting
  DEFAULT_LABEL = "unnamed"

  class Base
    def render
      raise NotImplementedError
    end
  end

  class Card < Base
    def render
      DEFAULT_LABEL.upcase
    end
  end
end

class Formatter < Reporting::Base
  def self.build
    new
  end
end

make_formatter = ->(label) { Formatter.build }
