# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.318"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.318/sshtermy_darwin_arm64"
      sha256 "0a5f2b78be4c04934423ece2f6516503c8ec661f40a82f339b4544c91f085514"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.318/sshtermy_darwin_amd64"
      sha256 "e5a712ac39a716c9fdc2c3e6f3287dddb4bfbb85792612e36c7732216a51bf4c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.318/sshtermy_linux_arm64"
      sha256 "a60eff2ded15fc6d64ed78e74cd11049476285fe51a35b506f725427734d952e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.318/sshtermy_linux_amd64"
      sha256 "ad5255a56bb5d68c6fdc7bd1918b889cec3378ddb10b798a7620c391bdd6952d"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
