# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.350"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.350/sshtermy_darwin_arm64"
      sha256 "82de5b1be97ffc47460b787d344a59a8d76b8732cdec2b130fd8293121c1f6ff"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.350/sshtermy_darwin_amd64"
      sha256 "59a1cca562e2b5f6e1a6a9b437c45e4e6d4649b262b56b8b652bb6ad0bf1b039"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.350/sshtermy_linux_arm64"
      sha256 "113a83a8fa881202b94bc30644af3d66c654b08f24855a3cc372fcf3d82e9073"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.350/sshtermy_linux_amd64"
      sha256 "6f1b15e366082792ea39038b8181190709091b609d5212551f7a42a95555a817"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
