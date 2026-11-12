# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.370"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.370/sshtermy_darwin_arm64"
      sha256 "b55acd3cdd50693d967b60894bf2ad37e43c1e0ad0ab56ae9cec80233689a905"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.370/sshtermy_darwin_amd64"
      sha256 "5374d8ae55a35997bb18813eaea479f58d76f8f5c885fcd54be7f6e26fa95d0b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.370/sshtermy_linux_arm64"
      sha256 "3bdf0099540cbaa731b9a2e5808e46cfffbc84d1cb1c1cb95f4d6c4f3fc54a6f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.370/sshtermy_linux_amd64"
      sha256 "1c90da63710a295b8852646cb0b528bea235fd5e973b3e8b21663c1ad5067807"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
