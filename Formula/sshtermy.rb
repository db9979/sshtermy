# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.228"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.228/sshtermy_darwin_arm64"
      sha256 "b315de16c8f2520c7654e804f8864abc495869aab245bb0d489892471fc67bf3"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.228/sshtermy_darwin_amd64"
      sha256 "122ff56aaccfdce1eb11e43b1952617024d2659e8bb07263576a6a9dcd284a26"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.228/sshtermy_linux_arm64"
      sha256 "a36ed1259664dfd30142c12fccc7a6e0fa7ba8f44ae7ff48bbabb287579ecf4e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.228/sshtermy_linux_amd64"
      sha256 "ab066cb900a7cb7cde3810978afa00d26f5f3a7047d3e9bea58f322e89ccb3f7"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
