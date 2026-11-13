# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.392"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.392/sshtermy_darwin_arm64"
      sha256 "6acf1b5ed8269f9d46353c6ec27d022acc559983a80986f9765b6308eed16ee8"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.392/sshtermy_darwin_amd64"
      sha256 "334c9426581cdaca866e9dc4a5f41d3031b0a38b90ed125d29b553500bff315b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.392/sshtermy_linux_arm64"
      sha256 "be4b7794a2e316cf1e548bf818622ac1d82a5b3a63b31b234dc151ce1db30958"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.392/sshtermy_linux_amd64"
      sha256 "27cb2a2a07305b84007214028d50247f508f5ab68a22dcb839b1cac037d663ae"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
