# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.177"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.177/sshtermy_darwin_arm64"
      sha256 "f2f8e695eeb5e2fffce5f9d2fed71bf4c5ac3386da9ec0ca7e4b97c893678ba4"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.177/sshtermy_darwin_amd64"
      sha256 "4a99dc77d7f3652db1318adebee35996fe588ef95d3b59a8feb33381fc15e0db"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.177/sshtermy_linux_arm64"
      sha256 "4893afc41e57c2f4f94bccdc8fee1ebd80603822d8e9118b1db61d334cfe9488"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.177/sshtermy_linux_amd64"
      sha256 "8228379320288d7a7a34f9a6b6b4a699790aa599ea742d667b8b7372fdca3287"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
