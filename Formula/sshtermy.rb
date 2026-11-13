# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.390"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.390/sshtermy_darwin_arm64"
      sha256 "62b64e2a776a8852e9129719af2e6347fe05ab9ac2bf1dad84867cde2d9ba075"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.390/sshtermy_darwin_amd64"
      sha256 "b45b863eb6527a70284ae11a7942f1416c460d5dba83529c61b86a704f46e37c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.390/sshtermy_linux_arm64"
      sha256 "341537e76c95c07ccc8cd360ab18b7533b5da85b4ffda51f75833415d093730d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.390/sshtermy_linux_amd64"
      sha256 "08887aef88596963abc2364b0b786c139f04d78782bcfe5bd4eaa7989374763a"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
