# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.372"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.372/sshtermy_darwin_arm64"
      sha256 "f5f9790c7adb54927ddd0330f6790ca855a68d9530cf85ddf6b4b20d3a02b876"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.372/sshtermy_darwin_amd64"
      sha256 "1456566050a1ac0a8e1897d665ede5fbd03e0619ef69b58193f798b77c687997"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.372/sshtermy_linux_arm64"
      sha256 "75a56d61c634053b56bf76af891decced091045414c6bac2b18dff707b5efaa5"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.372/sshtermy_linux_amd64"
      sha256 "617fb689f4c8b37bb048b6b829130153fdf77d545f12ceb13273f34d9b961c35"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
