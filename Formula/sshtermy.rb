# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.358"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.358/sshtermy_darwin_arm64"
      sha256 "c007cfd78ec0dc76eefd9c429322fda325e9ee50e55ef699f8096c644a503429"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.358/sshtermy_darwin_amd64"
      sha256 "08b52285e8882e52477810bb7788feb1b659fba251c3a50aa78ce4f258ed2131"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.358/sshtermy_linux_arm64"
      sha256 "823117ad66dc994e7740cc40d8be15ba553f939cd1cbaa10b4aec983b73bb2a8"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.358/sshtermy_linux_amd64"
      sha256 "9450e4997e5081020785d759868a7427b0701539e4bf4d46cde79f3192c93554"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
