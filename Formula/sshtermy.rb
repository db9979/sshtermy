# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.248"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.248/sshtermy_darwin_arm64"
      sha256 "4bb62e060769b6cc171f640d8ada61394f59e3c5fa4b074997941194c395d934"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.248/sshtermy_darwin_amd64"
      sha256 "a47f5a6f96687316c48a76ae518ce30d370196f7077ba6fcd4f2b32f7b931fe8"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.248/sshtermy_linux_arm64"
      sha256 "fe6fa9534e24a2d96f488ac9cb20ef401ec58c326871638177a38e56416fd666"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.248/sshtermy_linux_amd64"
      sha256 "f7f70d1325502529a4e6388df200847f33de86694dae7ccd6b0ce247050c7472"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
