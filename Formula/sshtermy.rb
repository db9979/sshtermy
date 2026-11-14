# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.407"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.407/sshtermy_darwin_arm64"
      sha256 "c9a6a724e5742d69d4be3454831907bf923fdb5f93e88c27b89a7542a0c19318"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.407/sshtermy_darwin_amd64"
      sha256 "0835ff3b78d421f1d010e36d0a16df1e11f615490e30ebc510bb7e2d7d4f8598"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.407/sshtermy_linux_arm64"
      sha256 "511ecc5f999190e714bb35ed8c71a72219eaf65c5d8644158cb5b55b1252e62a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.407/sshtermy_linux_amd64"
      sha256 "02b170647a4a79b59b129d7b7c5cf3cc1dc3008b34dfc68c7b4dbe697bd934ec"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
