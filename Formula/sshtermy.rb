# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.362"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.362/sshtermy_darwin_arm64"
      sha256 "343d21a8dac3b4e49120e4afb4b9d75f8f4d30ee5c63f3c6a8dc72555fe5094a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.362/sshtermy_darwin_amd64"
      sha256 "31e458865920bb65e1e7573eafedc6e99f6efc8b7de98fef55c2bfc66abb2452"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.362/sshtermy_linux_arm64"
      sha256 "ede5bab4cb595f9759e196a3a51a522611507e0c17a4d6de20a4635470a86677"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.362/sshtermy_linux_amd64"
      sha256 "9d5179f382eebe6d8b6747e62b2c34d9d7f96e95d8a85fa6eb11f667d338e2e2"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
